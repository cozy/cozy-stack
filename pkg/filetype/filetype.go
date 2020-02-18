package filetype

import (
	"bytes"
	"io"
	"mime"
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
	ftype "github.com/h2non/filetype"
)

// DefaultType is the type used when we can't know/guess the filetype.
const DefaultType = "application/octet-stream"

// ByExtension calls mime.TypeByExtension, and removes optional parameters, to
// keep only the type and subtype.
// Example: text/html
func ByExtension(ext string) string {
	if ext == ".url" {
		return consts.ShortcutMimeType
	}
	mimeParts := strings.SplitN(mime.TypeByExtension(ext), ";", 2)
	return strings.TrimSpace(mimeParts[0])
}

// Match returns the mime-type (no charset) if it can guess from the first
// bytes, or the default content-type else.
func Match(buf []byte) string {
	mimetype := DefaultType
	if kind, err := ftype.Match(buf); err == nil {
		mimetype = kind.MIME.Value
	}
	return mimetype
}

// FromReader takes a reader, sniffs the beginning of it, and returns the
// mime-type (no charset) and a new reader that's the concatenation of the
// bytes sniffed and the remaining reader.
func FromReader(r io.Reader) (string, io.Reader) {
	var buf bytes.Buffer
	_, err := io.Copy(&buf, io.LimitReader(r, 512))
	if err != nil {
		return DefaultType, io.MultiReader(&buf, errReader{err})
	}
	return Match(buf.Bytes()), io.MultiReader(&buf, r)
}

// errReader is an io.Reader which just returns err.
type errReader struct{ err error }

func (er errReader) Read([]byte) (int, error) { return 0, er.err }
