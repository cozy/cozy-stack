package webdav

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// handleProppatch implements RFC 4918 §9.2 PROPPATCH using Strategy B:
// all dead-property writes are rejected with 403 Forbidden in a 207
// Multi-Status response. This is the minimal Class 1 implementation —
// dead-properties persistence is a v2 requirement (ADV-V2-02).
//
// Control flow:
//  1. Read and parse the request body as <D:propertyupdate> XML.
//     Missing body → 400. Malformed XML → 400.
//  2. Validate the request path and look up the resource in the VFS.
//     Unknown path → 404.
//  3. Check the caller's permission with middlewares.AllowVFS.
//     Out-of-scope → 403.
//  4. Build a 207 Multi-Status response rejecting all proposed property
//     changes with "HTTP/1.1 403 Forbidden". Litmus accepts this as
//     "server rejects dead properties" for a Class 1 server.
//
// The 207 body format follows the convention from propfind.go:
// hand-written root element with xmlns:D="DAV:" so that children reuse
// the D: prefix without redundant namespace declarations.
func handleProppatch(c echo.Context) error {
	r := c.Request()

	// 1. Read body — PROPPATCH requires a body per RFC 4918 §9.2
	if r.ContentLength == 0 {
		return sendWebDAVError(c, http.StatusBadRequest, "bad-request")
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 256*1024))
	if err != nil || len(bodyBytes) == 0 {
		return sendWebDAVError(c, http.StatusBadRequest, "bad-request")
	}

	// Parse and validate XML — collect all proposed property names so we
	// can mirror them back in the 403 propstat.
	propNames, err := parseProppatchProps(bodyBytes)
	if err != nil {
		return sendWebDAVError(c, http.StatusBadRequest, "bad-request")
	}

	// 2. Path resolution
	rawParam := c.Param("*")
	vfsPath, err := davPathToVFSPath(rawParam)
	if err != nil {
		auditLog(c, "proppatch path rejected", rawParam)
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

	// 4. Permission check
	var fetcher vfs.Fetcher
	if dirDoc != nil {
		fetcher = dirDoc
	} else {
		fetcher = fileDoc
	}
	if err := middlewares.AllowVFS(c, permission.PUT, fetcher); err != nil {
		auditLog(c, "proppatch out-of-scope", vfsPath)
		return sendWebDAVError(c, http.StatusForbidden, "forbidden")
	}

	// 5. Build 207 Multi-Status rejecting all proposed properties with 403.
	// We use the request URI as the href so clients can correlate the response.
	requestURI := r.RequestURI
	respBody := buildProppatch403Response(requestURI, propNames)

	h := c.Response().Header()
	h.Set(echo.HeaderContentType, `application/xml; charset="utf-8"`)
	h.Set(echo.HeaderContentLength, strconv.Itoa(len(respBody)))
	c.Response().WriteHeader(http.StatusMultiStatus)
	_, werr := c.Response().Write(respBody)
	return werr
}

// propName holds the namespace URI and local name of a single XML property.
type propName struct {
	NS    string
	Local string
}

// parseProppatchProps extracts the list of property names from a
// <D:propertyupdate> XML body. It handles both <D:set> and <D:remove>
// blocks. Returns an error if the XML is malformed.
func parseProppatchProps(data []byte) ([]propName, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var props []propName
	inProp := false // true while inside a <D:prop> element

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name
			// <D:prop> marks the start of individual property elements
			if name.Space == "DAV:" && name.Local == "prop" {
				inProp = true
				continue
			}
			if inProp {
				// Each child of <D:prop> is a property being set/removed
				props = append(props, propName{NS: name.Space, Local: name.Local})
			}
		case xml.EndElement:
			name := t.Name
			if name.Space == "DAV:" && name.Local == "prop" {
				inProp = false
			}
		}
	}
	return props, nil
}

// buildProppatch403Response constructs the 207 Multi-Status body that
// rejects all proposed property changes with 403 Forbidden.
//
// Per RFC 4918 §9.2.2 a server MUST respond with a 403 for each property
// it will not accept. The simplest conformant form is a single <D:propstat>
// containing all rejected properties with <D:status>HTTP/1.1 403 Forbidden</D:status>.
func buildProppatch403Response(href string, props []propName) []byte {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	buf.WriteString("\n")
	buf.WriteString(`<D:multistatus xmlns:D="DAV:">`)
	buf.WriteString(`<D:response>`)
	buf.WriteString(`<D:href>`)
	xml.EscapeText(&buf, []byte(href)) //nolint:errcheck
	buf.WriteString(`</D:href>`)
	buf.WriteString(`<D:propstat>`)
	buf.WriteString(`<D:prop>`)

	// Emit an empty element for each requested property using its original
	// namespace. Properties without a namespace are emitted without a prefix.
	nsIndex := 0
	nsPrefixes := map[string]string{"DAV:": "D"}
	for _, p := range props {
		ns := p.NS
		prefix, ok := nsPrefixes[ns]
		if !ok {
			nsIndex++
			prefix = "ns" + strconv.Itoa(nsIndex)
			nsPrefixes[ns] = prefix
			// Emit namespace declaration on the first element using this NS
			buf.WriteString(`<`)
			buf.WriteString(prefix)
			buf.WriteString(`:`)
			buf.WriteString(p.Local)
			buf.WriteString(` xmlns:`)
			buf.WriteString(prefix)
			buf.WriteString(`="`)
			xml.EscapeText(&buf, []byte(ns)) //nolint:errcheck
			buf.WriteString(`"/>`)
		} else {
			buf.WriteString(`<`)
			buf.WriteString(prefix)
			buf.WriteString(`:`)
			buf.WriteString(p.Local)
			buf.WriteString(`/>`)
		}
	}

	buf.WriteString(`</D:prop>`)
	buf.WriteString(`<D:status>HTTP/1.1 403 Forbidden</D:status>`)
	buf.WriteString(`</D:propstat>`)
	buf.WriteString(`</D:response>`)
	buf.WriteString(`</D:multistatus>`)

	return buf.Bytes()
}
