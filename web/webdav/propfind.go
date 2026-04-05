package webdav

import (
	"crypto/md5"
	"encoding/binary"
	"errors"
	"net/http"
	"os"
	"path"
	"strconv"

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

// davFilesPrefix is the URL-space prefix clients see. Every <D:href> in
// a PROPFIND response is built relative to this root.
const davFilesPrefix = "/dav/files"

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

	// 2. Path resolution — security boundary, runs BEFORE any VFS call
	rawParam := c.Param("*")
	vfsPath, err := davPathToVFSPath(rawParam)
	if err != nil {
		auditLog(c, "propfind path rejected", rawParam)
		return sendWebDAVError(c, http.StatusForbidden, "forbidden")
	}

	// 3. VFS lookup
	inst := middlewares.GetInstance(c)
	dirDoc, fileDoc, err := inst.VFS().DirOrFileByPath(vfsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sendWebDAVError(c, http.StatusNotFound, "not-found")
		}
		return err
	}

	// 4. Permission check — AUTH-05
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

	// 5. Build responses
	responses := make([]Response, 0, 1)
	if fileDoc != nil {
		responses = append(responses, buildResponseForFile(fileDoc, vfsPath))
	} else {
		responses = append(responses, buildResponseForDir(dirDoc, vfsPath))
		if depth == "1" {
			if err := streamChildren(inst.VFS(), dirDoc, vfsPath, &responses); err != nil {
				return err
			}
		}
	}

	// 6. Marshal and send with explicit Content-Length
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

// streamChildren iterates the immediate children of dir via DirIterator
// (batched by propfindDirIteratorBatch) and appends one Response per child
// into out. It does not buffer the full listing — memory is bounded by
// the batch size.
func streamChildren(fs vfs.VFS, dir *vfs.DirDoc, dirVFSPath string, out *[]Response) error {
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
			*out = append(*out, buildResponseForDir(d, childVFSPath))
		case f != nil:
			childVFSPath := path.Join(dirVFSPath, f.DocName)
			*out = append(*out, buildResponseForFile(f, childVFSPath))
		}
	}
}

// buildResponseForDir returns a <D:response> carrying the 9 live
// properties for a directory. Collections MUST carry a trailing slash on
// their href per RFC 4918 §5.2.
//
// Directories have no MD5Sum, so the ETag is derived deterministically
// from DocID + UpdatedAt (pitfall 5 in 01-RESEARCH.md). The content-length
// and content-type properties are omitted (their struct fields are
// zero-valued and the encoder honours omitempty).
func buildResponseForDir(dir *vfs.DirDoc, vfsPath string) Response {
	href := davFilesPrefix + vfsPath
	if href == davFilesPrefix {
		href = davFilesPrefix + "/"
	} else if href[len(href)-1] != '/' {
		href += "/"
	}

	prop := Prop{
		ResourceType:    ResourceType{Collection: &struct{}{}},
		DisplayName:     dir.DocName,
		GetLastModified: buildLastModified(dir.UpdatedAt),
		GetETag:         etagForDir(dir),
		CreationDate:    buildCreationDate(dir.CreatedAt),
		SupportedLock:   &SupportedLock{},
		LockDiscovery:   &LockDiscovery{},
	}

	return Response{
		Href: href,
		Propstat: []Propstat{{
			Prop:   prop,
			Status: "HTTP/1.1 200 OK",
		}},
	}
}

// buildResponseForFile returns a <D:response> carrying the 9 live
// properties for a file. The href is the URL-space path with no trailing
// slash.
func buildResponseForFile(file *vfs.FileDoc, vfsPath string) Response {
	href := davFilesPrefix + vfsPath

	mime := file.Mime
	if mime == "" {
		mime = "application/octet-stream"
	}

	prop := Prop{
		ResourceType:     ResourceType{}, // no <D:collection/> on a file
		DisplayName:      file.DocName,
		GetLastModified:  buildLastModified(file.UpdatedAt),
		GetETag:          buildETag(file.MD5Sum),
		GetContentLength: file.ByteSize,
		GetContentType:   mime,
		CreationDate:     buildCreationDate(file.CreatedAt),
		SupportedLock:    &SupportedLock{},
		LockDiscovery:    &LockDiscovery{},
	}

	return Response{
		Href: href,
		Propstat: []Propstat{{
			Prop:   prop,
			Status: "HTTP/1.1 200 OK",
		}},
	}
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
