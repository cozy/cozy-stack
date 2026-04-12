package webdav

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// propfindDirIteratorBatch is the CouchDB fetch batch size used when
// streaming Depth:1 children. CONTEXT.md overrides the Cozy default of
// 100 with 200 to better balance per-batch latency against the per-page
// request count. The upper bound enforced by model/vfs is 256.
const propfindDirIteratorBatch = 200

// davFilesPrefix is the canonical URL-space prefix for the /dav/files/ route.
const davFilesPrefix = "/dav/files"

// davNextcloudPrefix is the URL-space prefix for the Nextcloud-compatible route.
const davNextcloudPrefix = "/remote.php/webdav"

// hrefPrefixFor derives the URL prefix that should appear in <D:href> elements
// based on the incoming request path. When a request arrives via the Nextcloud
// compatibility route (/remote.php/webdav/...) the href elements must use
// /remote.php/webdav as their root so that clients can round-trip the paths
// they receive. All other requests (including /dav/files/...) use davFilesPrefix.
//
// This fixes the litmus propfind_d0 WARNING: "response href for wrong resource"
// which fires when PROPFIND over /remote.php/webdav/ returns /dav/files/ hrefs.
func hrefPrefixFor(c echo.Context) string {
	reqPath := c.Request().URL.Path
	if strings.HasPrefix(reqPath, davNextcloudPrefix) {
		return davNextcloudPrefix
	}
	return davFilesPrefix
}

// handlePropfind implements RFC 4918 §9.1 PROPFIND for Depth:0 and Depth:1.
//
// Control flow:
//  1. Parse Depth header. "infinity" is rejected with 403
//     <D:propfind-finite-depth/> and a WARN audit log. Missing defaults to
//     "1" (the RFC default is technically infinity but for safety we treat
//     absence as collection-level only).
//  2. Normalise the path through davPathToVFSPath — every traversal /
//     encoded-escape / null-byte variant is rejected as 403 before any VFS
//     access.
//  3. Look up the resource with DirOrFileByPath. Missing → 404.
//  4. Check the caller's permission with middlewares.AllowVFS. Out-of-scope
//     → 403 with WARN audit log.
//  5. Build the response list. For files and Depth:0 directories that is
//     exactly one <D:response>. For Depth:1 directories we stream children
//     through the VFS DirIterator (ByFetch=200) without buffering the full
//     listing in memory.
//  6. Marshal via marshalMultistatus (from plan 01-02) — this returns []byte
//     so we can set Content-Length before writing the status header as
//     required by SEC-05 / Finder strictness.
func handlePropfind(c echo.Context) error {
	// 1. Depth header
	depth := c.Request().Header.Get("Depth")
	switch depth {
	case "":
		depth = "1"
	case "0", "1":
		// ok
	case "infinity", "Infinity":
		auditLog(c, "propfind depth infinity rejected", c.Param("*"))
		return sendWebDAVError(c, http.StatusForbidden, "propfind-finite-depth")
	default:
		return sendWebDAVError(c, http.StatusBadRequest, "bad-depth")
	}

	// 2. XML body validation — RFC 4918 §9.1 requires 400 for unparseable bodies.
	// An empty body is valid (treated as allprop). A non-empty body that is not
	// well-formed XML must be rejected.
	//
	// We consume the body entirely only for validation; we do not act on the
	// parsed content (full <D:prop> filtering is a v2 feature). This addresses
	// litmus propfind_invalid ("non-well-formed XML body") and propfind_invalid2
	// ("invalid namespace declaration").
	if r := c.Request(); r.ContentLength != 0 {
		bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
		if err != nil {
			return sendWebDAVError(c, http.StatusBadRequest, "bad-request")
		}
		if len(bodyBytes) > 0 {
			if err := validatePropfindXML(bodyBytes); err != nil {
				return sendWebDAVError(c, http.StatusBadRequest, "bad-request")
			}
		}
	}

	// 3. Path resolution — security boundary, runs BEFORE any VFS call
	rawParam := c.Param("*")
	vfsPath, err := davPathToVFSPath(rawParam)
	if err != nil {
		auditLog(c, "propfind path rejected", rawParam)
		return sendWebDAVError(c, http.StatusForbidden, "forbidden")
	}

	// 4. VFS lookup
	inst := middlewares.GetInstance(c)
	dirDoc, fileDoc, err := inst.VFS().DirOrFileByPath(vfsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sendWebDAVError(c, http.StatusNotFound, "not-found")
		}
		return err
	}

	// 5. Permission check — AUTH-05
	var fetcher vfs.Fetcher
	if dirDoc != nil {
		fetcher = dirDoc
	} else {
		fetcher = fileDoc
	}
	if err := middlewares.AllowVFS(c, permission.GET, fetcher); err != nil {
		auditLog(c, "propfind out-of-scope", vfsPath)
		return sendWebDAVError(c, http.StatusForbidden, "forbidden")
	}

	// 6. Determine href prefix — depends on which route handled this request
	// (dav/files or remote.php/webdav). Fixes litmus propfind_d0 warning on
	// the Nextcloud route ("response href for wrong resource").
	hrefPrefix := hrefPrefixFor(c)

	// 7. Build responses
	domain := inst.Domain
	responses := make([]Response, 0, 1)
	if fileDoc != nil {
		responses = append(responses, buildResponseForFileWithPrefix(fileDoc, vfsPath, hrefPrefix))
	} else {
		responses = append(responses, buildResponseForDirWithPrefix(dirDoc, vfsPath, hrefPrefix))
		if depth == "1" {
			if err := streamChildrenWithPrefix(inst.VFS(), dirDoc, vfsPath, hrefPrefix, &responses); err != nil {
				return err
			}
		}
	}

	// Inject any dead (custom) properties stored by PROPPATCH into each response.
	// For Depth:1, only the container itself gets its dead properties here;
	// each child's VFS path is embedded in its Response.Href.
	for i := range responses {
		rspVFSPath := hrefToVFSPath(responses[i].Href, hrefPrefix)
		if dp := buildDeadPropsXML(domain, rspVFSPath); len(dp) > 0 {
			for j := range responses[i].Propstat {
				responses[i].Propstat[j].Prop.DeadPropsXML = dp
			}
		}
	}

	// 8. Marshal and send with explicit Content-Length
	body, err := marshalMultistatus(responses)
	if err != nil {
		return err
	}
	h := c.Response().Header()
	h.Set(echo.HeaderContentType, `application/xml; charset="utf-8"`)
	h.Set(echo.HeaderContentLength, strconv.Itoa(len(body)))
	c.Response().WriteHeader(http.StatusMultiStatus)
	_, werr := c.Response().Write(body)
	return werr
}

// validatePropfindXML parses the given bytes as XML and returns an error if
// the XML is not well-formed or uses invalid namespace declarations.
//
// Checks performed:
//  1. Well-formedness via xml.Decoder (catches unclosed tags, bad encoding, etc.)
//  2. Invalid namespace bindings: per XML Namespaces 1.0, a non-default namespace
//     prefix must NOT be bound to an empty string URI. Go's xml.Decoder accepts
//     xmlns:foo="" silently, but such requests should be rejected per RFC 4918 §8.3
//     and the litmus propfind_invalid2 test.
func validatePropfindXML(data []byte) error {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		// Check for invalid namespace declarations on start elements.
		// xmlns:foo="" is invalid — a non-default prefix must not map to "".
		if se, ok := tok.(xml.StartElement); ok {
			for _, attr := range se.Attr {
				if attr.Name.Space == "xmlns" && attr.Name.Local != "" && attr.Value == "" {
					return fmt.Errorf("invalid namespace binding: xmlns:%s is empty", attr.Name.Local)
				}
			}
		}
	}
}

// streamChildren iterates the immediate children of dir via DirIterator
// (batched by propfindDirIteratorBatch) and appends one Response per child
// into out. It does not buffer the full listing — memory is bounded by
// the batch size.
func streamChildren(fs vfs.VFS, dir *vfs.DirDoc, dirVFSPath string, out *[]Response) error {
	return streamChildrenWithPrefix(fs, dir, dirVFSPath, davFilesPrefix, out)
}

// streamChildrenWithPrefix is like streamChildren but uses hrefPrefix for the
// hrefs in each child response. Used by handlePropfind on the Nextcloud route.
func streamChildrenWithPrefix(fs vfs.VFS, dir *vfs.DirDoc, dirVFSPath, hrefPrefix string, out *[]Response) error {
	iter := fs.DirIterator(dir, &vfs.IteratorOptions{ByFetch: propfindDirIteratorBatch})
	for {
		d, f, err := iter.Next()
		if errors.Is(err, vfs.ErrIteratorDone) {
			return nil
		}
		if err != nil {
			return err
		}
		switch {
		case d != nil:
			childVFSPath := path.Join(dirVFSPath, d.DocName)
			*out = append(*out, buildResponseForDirWithPrefix(d, childVFSPath, hrefPrefix))
		case f != nil:
			childVFSPath := path.Join(dirVFSPath, f.DocName)
			*out = append(*out, buildResponseForFileWithPrefix(f, childVFSPath, hrefPrefix))
		}
	}
}

// propstatOK is the canonical success status line for a <D:propstat>
// block — RFC 4918 §14.22 mandates the literal "HTTP/1.1 200 OK" form.
const propstatOK = "HTTP/1.1 200 OK"

// hrefForDir builds the URL-space href for a directory using the given prefix.
// Collections MUST carry a trailing slash per RFC 4918 §5.2 — clients use it
// to distinguish a collection from a same-named file without inspecting
// <D:resourcetype>.
func hrefForDir(vfsPath string) string {
	return hrefForDirWithPrefix(vfsPath, davFilesPrefix)
}

func hrefForDirWithPrefix(vfsPath, prefix string) string {
	href := prefix + vfsPath
	if href == prefix || href[len(href)-1] != '/' {
		href += "/"
	}
	return href
}

// hrefForFile builds the URL-space href for a file — the VFS path verbatim
// under the dav root, with no trailing slash.
func hrefForFile(vfsPath string) string {
	return davFilesPrefix + vfsPath
}

func hrefForFileWithPrefix(vfsPath, prefix string) string {
	return prefix + vfsPath
}

// baseProps fills the live properties shared by files and directories:
// displayname, getlastmodified, creationdate, and the empty supportedlock
// / lockdiscovery stubs. Callers layer the type-specific fields
// (resourcetype, getetag, getcontentlength, getcontenttype) on top.
func baseProps(name string, createdAt, updatedAt time.Time) Prop {
	return Prop{
		DisplayName:     name,
		GetLastModified: buildLastModified(updatedAt),
		CreationDate:    buildCreationDate(createdAt),
		SupportedLock:   &SupportedLock{},
		LockDiscovery:   &LockDiscovery{},
	}
}

// buildResponseForDir returns a <D:response> carrying the 9 live
// properties for a directory. Directories have no MD5Sum so the ETag is
// derived deterministically from DocID + UpdatedAt (pitfall 5 in
// 01-RESEARCH.md). The content-length and content-type properties are
// omitted (zero-valued fields with omitempty struct tags).
func buildResponseForDir(dir *vfs.DirDoc, vfsPath string) Response {
	return buildResponseForDirWithPrefix(dir, vfsPath, davFilesPrefix)
}

// buildResponseForDirWithPrefix is like buildResponseForDir but uses the
// given hrefPrefix instead of the default davFilesPrefix. Used by
// handlePropfind to emit correct hrefs on the Nextcloud route.
func buildResponseForDirWithPrefix(dir *vfs.DirDoc, vfsPath, hrefPrefix string) Response {
	prop := baseProps(dir.DocName, dir.CreatedAt, dir.UpdatedAt)
	prop.ResourceType = ResourceType{Collection: &struct{}{}}
	prop.GetETag = etagForDir(dir)

	return Response{
		Href: hrefForDirWithPrefix(vfsPath, hrefPrefix),
		Propstat: []Propstat{{
			Prop:   prop,
			Status: propstatOK,
		}},
	}
}

// buildResponseForFile returns a <D:response> carrying the 9 live
// properties for a file. Mime falls back to application/octet-stream per
// RFC 7231 §3.1.1.5 when the VFS has no stored content type.
func buildResponseForFile(file *vfs.FileDoc, vfsPath string) Response {
	return buildResponseForFileWithPrefix(file, vfsPath, davFilesPrefix)
}

// buildResponseForFileWithPrefix is like buildResponseForFile but uses the
// given hrefPrefix instead of the default davFilesPrefix. Used by
// handlePropfind to emit correct hrefs on the Nextcloud route.
func buildResponseForFileWithPrefix(file *vfs.FileDoc, vfsPath, hrefPrefix string) Response {
	mime := file.Mime
	if mime == "" {
		mime = "application/octet-stream"
	}

	prop := baseProps(file.DocName, file.CreatedAt, file.UpdatedAt)
	prop.ResourceType = ResourceType{} // no <D:collection/> on a file
	prop.GetETag = buildETag(file.MD5Sum)
	prop.GetContentLength = file.ByteSize
	prop.GetContentType = mime

	return Response{
		Href: hrefForFileWithPrefix(vfsPath, hrefPrefix),
		Propstat: []Propstat{{
			Prop:   prop,
			Status: propstatOK,
		}},
	}
}

// hrefToVFSPath extracts the VFS path from an href by stripping the given
// prefix and normalising the trailing slash. Used when injecting dead
// properties into PROPFIND responses.
func hrefToVFSPath(href, prefix string) string {
	vfsPath := strings.TrimPrefix(href, prefix)
	vfsPath = strings.TrimRight(vfsPath, "/")
	if vfsPath == "" {
		vfsPath = "/"
	}
	return vfsPath
}

// buildDeadPropsXML returns the raw XML bytes for all dead properties stored
// under (domain, vfsPath). The bytes are suitable for injection into a
// <D:prop> element via the DeadPropsXML ",innerxml" field.
//
// Each property element carries its own namespace declaration. Since these
// are NOT self-closing elements (they have text content), the namespace
// scope covers the full open..close range and neon/libxml2 parses them
// correctly (unlike self-closing PROPPATCH echo elements).
//
// Returns nil if no dead properties exist for the resource.
func buildDeadPropsXML(domain, vfsPath string) []byte {
	stored := deadPropStore.listFor(domain, vfsPath)
	if len(stored) == 0 {
		return nil
	}

	var buf strings.Builder
	for _, entry := range stored {
		ns := entry.k.namespace
		local := entry.k.local
		value := entry.v

		if ns == "" {
			// No namespace — emit without prefix
			fmt.Fprintf(&buf, `<%s>%s</%s>`, local, value, local)
		} else {
			// Use a per-element namespace declaration with dp: prefix.
			// Non-self-closing elements keep the namespace in scope
			// for both the value and the closing tag.
			fmt.Fprintf(&buf, `<dp:%s xmlns:dp="%s">%s</dp:%s>`, local, ns, value, local)
		}
	}

	return []byte(buf.String())
}

// etagForDir returns a deterministic quoted ETag for a directory. The
// VFS does not store an md5sum for directories, so we synthesise one
// from (DocID, UpdatedAt.UnixNano). This is stable across reads as long
// as the directory's metadata hasn't changed, which is the contract
// clients expect for change-detection.
func etagForDir(dir *vfs.DirDoc) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(dir.UpdatedAt.UnixNano()))
	sum := md5.Sum(append([]byte(dir.DocID), buf[:]...))
	return buildETag(sum[:])
}
