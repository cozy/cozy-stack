package utils

import (
	"io"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
)

// ServeContent replies to the request using the content in the provided
// reader. The Content-Length and Content-Type headers are added with the
// provided values.
func ServeContent(w http.ResponseWriter, r *http.Request, contentType string, size int64, content io.Reader) {
	h := w.Header()
	if size > 0 {
		h.Set("Content-Length", strconv.FormatInt(size, 10))
	}
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	w.WriteHeader(http.StatusOK)
	if r.Method != "HEAD" {
		_, _ = io.Copy(w, content)
	}
}

// CheckPreconditions evaluates request preconditions based only on the Etag
// values.
func CheckPreconditions(w http.ResponseWriter, r *http.Request, etag string) (done bool) {
	inm := r.Header.Get("If-None-Match")

	if inm != "" && etag != "" && checkIfNoneMatch(inm, etag) {
		writeNotModified(w)
		return true
	}

	return false
}

func checkIfNoneMatch(ifNoneMatch, definedETag string) (match bool) {
	buf := ifNoneMatch
	for {
		buf = textproto.TrimString(buf)
		if len(buf) == 0 {
			break
		}
		if buf[0] == ',' {
			buf = buf[1:]
		}
		if buf[0] == '*' {
			return true
		}
		etag, remain := scanETag(buf)
		if etag == "" {
			break
		}
		if etagWeakMatch(etag, definedETag) {
			return true
		}
		buf = remain
	}
	return false
}

// etagWeakMatch reports whether a and b match using weak ETag comparison.
// Assumes a and b are valid ETags.
// More at: https://www.rfc-editor.org/rfc/rfc9110#name-comparison-2
func etagWeakMatch(a, b string) bool {
	return strings.TrimPrefix(a, "W/") == strings.TrimPrefix(b, "W/")
}

// scanETag determines if a syntactically valid ETag is present at s. If so,
// the ETag and remaining text after consuming ETag is returned. Otherwise,
// it returns "", "".
func scanETag(s string) (etag string, remain string) {
	start := 0
	if len(s) >= 2 && s[0] == 'W' && s[1] == '/' {
		start = 2
	}
	if len(s[start:]) < 2 || s[start] != '"' {
		return "", ""
	}
	// ETag is either W/"text" or "text".
	// See RFC 7232 2.3.
	for i := start + 1; i < len(s); i++ {
		c := s[i]
		switch {
		// Character values allowed in ETags.
		case c == 0x21 || c >= 0x23 && c <= 0x7E || c >= 0x80:
		case c == '"':
			return s[:i+1], s[i+1:]
		default:
			return "", ""
		}
	}
	return "", ""
}

func writeNotModified(w http.ResponseWriter) {
	h := w.Header()
	delete(h, "Content-Type")
	delete(h, "Content-Length")
	w.WriteHeader(http.StatusNotModified)
}
