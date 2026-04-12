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

// handleProppatch implements RFC 4918 §9.2 PROPPATCH using an in-memory
// dead-property store (Strategy C). Properties are stored in memory per
// instance domain, not persisted to CouchDB. The store resets on server
// restart. Full CouchDB persistence is a v2 requirement (ADV-V2-02).
//
// Control flow:
//  1. Read and validate the request body as <D:propertyupdate> XML.
//     Missing body → 400. Malformed XML → 400.
//  2. Validate the request path and look up the resource in the VFS.
//     Unknown path → 404.
//  3. Check the caller's permission with middlewares.AllowVFS.
//     Out-of-scope → 403.
//  4. Apply <D:set> and <D:remove> operations to deadPropStore.
//  5. Return 207 Multi-Status with 200 OK for each processed property.
//
// The 207 body format follows propfind.go conventions (hand-written root
// with xmlns:D="DAV:", all custom namespaces on the <D:prop> ancestor).
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

	// Parse property updates — collect set and remove operations.
	ops, err := parseProppatchOps(bodyBytes)
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

	// 5. Apply operations to the in-memory dead-property store.
	domain := inst.Domain
	for _, op := range ops {
		key := deadPropKey{
			domain:    domain,
			vfsPath:   vfsPath,
			namespace: op.name.NS,
			local:     op.name.Local,
		}
		if op.remove {
			deadPropStore.remove(key)
		} else {
			deadPropStore.set(key, op.value)
		}
	}

	// 6. Build 207 Multi-Status with 200 OK for each processed operation.
	requestURI := r.RequestURI
	var allNames []propOp
	allNames = append(allNames, ops...)
	respBody := buildProppatch200Response(requestURI, allNames)

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

// propOp represents a single set or remove operation from a <D:propertyupdate>.
type propOp struct {
	name   propName
	value  string // raw inner XML for set; empty for remove
	remove bool   // true for <D:remove> operations
}

// parseProppatchOps parses a <D:propertyupdate> body and returns ordered
// list of set/remove operations. The order follows the order of appearance
// in the document (RFC 4918 §9.2 requires operations to be atomic and
// processed in document order).
func parseProppatchOps(data []byte) ([]propOp, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var ops []propOp

	type state int
	const (
		stateRoot  state = iota
		stateSet         // inside <D:set>
		stateRemove      // inside <D:remove>
		stateProp        // inside <D:prop>
	)

	st := stateRoot
	var setMode bool // true for set, false for remove (used when in stateProp)
	var depth int    // nesting depth inside a property element (0 = not in one)
	var curProp *propOp
	var valueBuf bytes.Buffer
	// prefixStack tracks the namespace prefix used at each nesting depth >= 2
	// (depth 1 is the property element itself, tracked via curProp.name.NS).
	// Index 0 = prefix used at depth 2, index 1 = depth 3, etc.
	var prefixStack []string
	// nsCounter assigns unique prefixes to each distinct namespace URI encountered
	// inside property values so nested elements with different namespaces don't
	// collide on the same "ns:" prefix.
	nsMap := make(map[string]string)
	nsCounter := 0

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
			switch {
			case depth > 0:
				// Inside a property value — collect raw XML.
				// Assign a stable prefix for each distinct namespace URI so that
				// the opening and closing tags always match.
				depth++
				var prefix string
				if t.Name.Space != "" {
					if p, ok := nsMap[t.Name.Space]; ok {
						prefix = p
					} else {
						nsCounter++
						prefix = "ns" + strconv.Itoa(nsCounter)
						nsMap[t.Name.Space] = prefix
					}
				}
				// Push onto prefix stack (slot = depth-2 since depth was already incremented)
				prefixStack = append(prefixStack, prefix)
				valueBuf.WriteString(`<`)
				if prefix != "" {
					valueBuf.WriteString(prefix)
					valueBuf.WriteString(`:`)
				}
				valueBuf.WriteString(t.Name.Local)
				if t.Name.Space != "" {
					valueBuf.WriteString(` xmlns:`)
					valueBuf.WriteString(prefix)
					valueBuf.WriteString(`="`)
					valueBuf.WriteString(t.Name.Space)
					valueBuf.WriteString(`"`)
				}
				valueBuf.WriteString(`>`)

			case st == stateRoot && t.Name.Space == "DAV:" && t.Name.Local == "set":
				st = stateSet
				setMode = true

			case st == stateRoot && t.Name.Space == "DAV:" && t.Name.Local == "remove":
				st = stateRemove
				setMode = false

			case (st == stateSet || st == stateRemove) && t.Name.Space == "DAV:" && t.Name.Local == "prop":
				st = stateProp

			case st == stateProp:
				// Start of a property element
				curProp = &propOp{
					name:   propName{NS: t.Name.Space, Local: t.Name.Local},
					remove: !setMode,
				}
				depth = 1
				valueBuf.Reset()
				prefixStack = prefixStack[:0]
				nsMap = make(map[string]string)
				nsCounter = 0
			}

		case xml.EndElement:
			switch {
			case depth > 1:
				// Closing a nested element inside a property value.
				// Pop the prefix stack to get the prefix used for the opening tag.
				stackIdx := len(prefixStack) - 1
				prefix := ""
				if stackIdx >= 0 {
					prefix = prefixStack[stackIdx]
					prefixStack = prefixStack[:stackIdx]
				}
				depth--
				valueBuf.WriteString(`</`)
				if prefix != "" {
					valueBuf.WriteString(prefix)
					valueBuf.WriteString(`:`)
				}
				valueBuf.WriteString(t.Name.Local)
				valueBuf.WriteString(`>`)

			case depth == 1:
				// Closing the property element itself
				depth = 0
				if curProp != nil {
					curProp.value = valueBuf.String()
					ops = append(ops, *curProp)
					curProp = nil
				}

			case t.Name.Space == "DAV:" && (t.Name.Local == "set" || t.Name.Local == "remove"):
				st = stateRoot

			case t.Name.Space == "DAV:" && t.Name.Local == "prop":
				st = stateSet // back to set/remove level; actual state restored by </set>/<remove>
			}

		case xml.CharData:
			if depth > 0 {
				xml.EscapeText(&valueBuf, []byte(t)) //nolint:errcheck
			}
		}
	}
	return ops, nil
}

// buildProppatch200Response constructs the 207 Multi-Status body that
// confirms all requested property changes were accepted with 200 OK.
//
// CRITICAL: All namespace declarations are collected and emitted on the
// <D:prop> element (the common ancestor). Individual property elements are
// self-closing without per-element xmlns declarations. This is required for
// compatibility with libxml2/neon — when a namespace is declared on a
// self-closing element (<ns1:foo xmlns:ns1="..."/>) the scope closes with
// the element, leaving subsequent sibling elements unable to use the prefix.
func buildProppatch200Response(href string, ops []propOp) []byte {
	// First pass: collect all unique non-DAV namespaces and assign prefixes.
	nsIndex := 0
	nsPrefixes := map[string]string{"DAV:": "D", "": ""}
	var nsList []struct{ prefix, uri string } // ordered for deterministic output
	for _, op := range ops {
		ns := op.name.NS
		if _, ok := nsPrefixes[ns]; !ok {
			nsIndex++
			prefix := "ns" + strconv.Itoa(nsIndex)
			nsPrefixes[ns] = prefix
			nsList = append(nsList, struct{ prefix, uri string }{prefix, ns})
		}
	}

	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	buf.WriteString("\n")
	buf.WriteString(`<D:multistatus xmlns:D="DAV:">`)
	buf.WriteString(`<D:response>`)
	buf.WriteString(`<D:href>`)
	xml.EscapeText(&buf, []byte(href)) //nolint:errcheck
	buf.WriteString(`</D:href>`)
	buf.WriteString(`<D:propstat>`)

	// Emit <D:prop> with all custom namespace declarations on the opening tag.
	buf.WriteString(`<D:prop`)
	for _, ns := range nsList {
		buf.WriteString(` xmlns:`)
		buf.WriteString(ns.prefix)
		buf.WriteString(`="`)
		xml.EscapeText(&buf, []byte(ns.uri)) //nolint:errcheck
		buf.WriteString(`"`)
	}
	buf.WriteString(`>`)

	// Second pass: emit property elements using the already-declared prefixes.
	for _, op := range ops {
		ns := op.name.NS
		prefix := nsPrefixes[ns]
		if prefix == "" {
			buf.WriteString(`<`)
			buf.WriteString(op.name.Local)
			buf.WriteString(`/>`)
		} else {
			buf.WriteString(`<`)
			buf.WriteString(prefix)
			buf.WriteString(`:`)
			buf.WriteString(op.name.Local)
			buf.WriteString(`/>`)
		}
	}

	buf.WriteString(`</D:prop>`)
	buf.WriteString(`<D:status>HTTP/1.1 200 OK</D:status>`)
	buf.WriteString(`</D:propstat>`)
	buf.WriteString(`</D:response>`)
	buf.WriteString(`</D:multistatus>`)

	return buf.Bytes()
}
