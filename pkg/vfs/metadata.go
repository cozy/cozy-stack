package vfs

import (
	"io"

	"github.com/xor-gate/goexif2/exif"
)

// TODO add tests

// Metadata is a list of metadata specific to each mimetype:
// id3 for music, exif for jpegs, etc.
type Metadata map[string]interface{}

// MetaExtractor is an interface for extracting metadata from a file
type MetaExtractor interface {
	io.WriteCloser
	Abort(error)
	Result() Metadata
}

// NewMetaExtractor returns an extractor for metadata if the mime type has one,
// or null else
func NewMetaExtractor(doc *FileDoc) *MetaExtractor {
	// TODO png, gif, etc.
	if doc.Mime == "image/jpg" {
		var e MetaExtractor = NewExifExtractor()
		return &e
	}
	return nil
}

// ExifExtractor is used to extract EXIF metadata from jpegs
type ExifExtractor struct {
	w *io.PipeWriter
	r *io.PipeReader
}

// NewExifExtractor returns an extractor for EXIF metadata
// TODO first, make it works
// TODO then, use a goroutine to avoid having the whole file in memory
func NewExifExtractor() *ExifExtractor {
	e := &ExifExtractor{}
	e.r, e.w = io.Pipe()
	return e
}

// Write is called to push some bytes to the extractor
func (e *ExifExtractor) Write(p []byte) (n int, err error) {
	return e.w.Write(p)
}

// Close is called when all the bytes has been pushed, to finalize the extraction
func (e *ExifExtractor) Close() error {
	return e.w.Close()
}

// Abort is called when the extractor can be discarded
func (e *ExifExtractor) Abort(err error) {
	e.w.CloseWithError(err)
}

// Result is called to get the extracted metadata
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
