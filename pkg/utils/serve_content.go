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
		h := w.Header()
		delete(h, "Content-Type")
		delete(h, "Content-Length")
		w.WriteHeader(http.StatusNotModified)
		return true
	}

	return false
}

func checkIfNoneMatch(ifNoneMatch, definedETag string) bool {
	values := strings.Split(ifNoneMatch, ",")

	for _, val := range values {
		val = textproto.TrimString(val)

		if val == "*" {
			return true
		}

		val = strings.TrimPrefix(val, "W/")

		if !strings.HasPrefix(val, `"`) || !strings.HasSuffix(val, `"`) {
			return false
		}

		// Remove the `"`
		etagContent := val[1 : len(val)-1]

		// Check that we have only valid runes.
		invalidRunIdx := strings.IndexFunc(etagContent, func(r rune) bool {
			return !(r == 0x21 || r >= 0x23 && r <= 0x7E || r >= 0x80)
		})

		if invalidRunIdx != -1 {
			return false
		}

		if val == strings.TrimPrefix(definedETag, "W/") {
			return true
		}
	}

	return false
}
