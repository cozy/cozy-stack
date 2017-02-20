package vfs

import (
	"io"

	"github.com/xor-gate/goexif2/exif"
)

// TODO add comments and tests

type Metadata map[string]interface{}

type MetaExtractor interface {
	io.WriteCloser
	Abort(error)
	Result() Metadata
}

func NewMetaExtractor(doc *FileDoc) *MetaExtractor {
	// TODO png, gif, etc.
	if doc.Mime == "image/jpg" {
		var e MetaExtractor = NewExifExtractor()
		return &e
	}
	return nil
}

type ExifExtractor struct {
	w *io.PipeWriter
	r *io.PipeReader
}

// TODO first, make it works
// TODO then, use a goroutine to avoid having the whole file in memory
func NewExifExtractor() *ExifExtractor {
	e := &ExifExtractor{}
	e.r, e.w = io.Pipe()
	return e
}

func (e *ExifExtractor) Write(p []byte) (n int, err error) {
	return e.w.Write(p)
}

func (e *ExifExtractor) Close() error {
	return e.w.Close()
}

func (e *ExifExtractor) Abort(err error) {
	e.w.CloseWithError(err)
}

func (e *ExifExtractor) Result() Metadata {
	m := Metadata{}
	x, err := exif.Decode(e.r)
	if err != nil {
		return m
	}
	// TODO add other metadata
	if dt, err := x.DateTime(); err == nil {
		m["datetime"] = dt
	}
	return m
}
