package fs

import (
	"io"
	"net/http"
	"net/textproto"
	"strings"
)

type h struct {
	prefix string
	assets map[string]*Asset
}

func Handler(prefix string, privateAssets ...string) http.Handler {
	files := make(map[string]*Asset)
	// TODO: the notion of context does not make sense in this example.
	Foreach(func(name, context string, f *Asset) {
		isPrivate := false
		for _, p := range privateAssets {
			if strings.HasPrefix(name, p) {
				isPrivate = true
				break
			}
		}
		if !isPrivate {
			files[name] = f
		}
	})
	return &h{
		prefix: prefix,
		assets: files,
	}
}

func (h *h) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	file := strings.TrimPrefix(r.URL.Path, h.prefix)
	f, ok := h.assets[file]
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	if inm := r.Header.Get("If-None-Match"); inm != "" {
		var match bool
		for {
			inm = textproto.TrimString(inm)
			if len(inm) == 0 {
				break
			}
			if inm[0] == ',' {
				inm = inm[1:]
			}
			if inm[0] == '*' {
				match = true
				break
			}
			etag, remain := scanETag(inm)
			if etag == "" {
				break
			}
			if etagWeakMatch(etag, f.Etag) {
				match = true
				break
			}
			inm = remain
		}
		if match {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	headers := w.Header()
	headers.Set("Content-Type", f.Mime)
	headers.Set("Content-Length", f.Size())
	headers.Set("Etag", f.Etag)
	headers.Set("Cache-Control", "no-cache, public")

	if r.Method == http.MethodGet {
		io.Copy(w, f.Reader())
	}
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

// etagWeakMatch reports whether a and b match using weak ETag comparison.
// Assumes a and b are valid ETags.
func etagWeakMatch(a, b string) bool {
	return strings.TrimPrefix(a, "W/") == strings.TrimPrefix(b, "W/")
}
