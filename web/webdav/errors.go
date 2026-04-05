package webdav

import (
	"bytes"
	"strconv"

	"github.com/labstack/echo/v4"
)

// buildErrorXML returns an RFC 4918 §8.7 error body carrying a single
// precondition / postcondition element. The root element declares the DAV:
// namespace with the D: prefix so clients can parse the body without
// negotiating XML namespaces on the fly — required for Windows
// Mini-Redirector compatibility (see STATE.md Architecture Decisions).
//
// condition is the local name of the condition element, e.g.
// "propfind-finite-depth", "lock-token-submitted", "forbidden".
func buildErrorXML(condition string) []byte {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	buf.WriteString(`<D:error xmlns:D="DAV:"><D:`)
	buf.WriteString(condition)
	buf.WriteString(`/></D:error>`)
	return buf.Bytes()
}

// sendWebDAVError writes an RFC 4918 XML error body to c with the given HTTP
// status and condition element. It sets Content-Type and Content-Length
// before calling WriteHeader — SEC-05 requires Content-Length on every
// non-2xx WebDAV response, and macOS / iOS clients break without it.
//
// Every non-2xx response from a WebDAV handler in later plans must go
// through this helper so that error bodies are consistent and
// audit-friendly.
func sendWebDAVError(c echo.Context, status int, condition string) error {
	body := buildErrorXML(condition)
	h := c.Response().Header()
	h.Set(echo.HeaderContentType, `application/xml; charset="utf-8"`)
	h.Set(echo.HeaderContentLength, strconv.Itoa(len(body)))
	c.Response().WriteHeader(status)
	_, err := c.Response().Write(body)
	return err
}
