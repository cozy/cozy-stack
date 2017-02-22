package vfs

import (
	"io"

	"github.com/cozy/goexif2/exif"
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
	w  *io.PipeWriter
	r  *io.PipeReader
	ch chan interface{}
}

// NewExifExtractor returns an extractor for EXIF metadata
func NewExifExtractor() *ExifExtractor {
	e := &ExifExtractor{}
	e.r, e.w = io.Pipe()
	e.ch = make(chan interface{})
	go e.Start()
	return e
}

// Start is used in a goroutine to start the metadata extraction
func (e *ExifExtractor) Start() {
	x, err := exif.Decode(e.r)
	if err != nil {
		e.ch <- err
	} else {
		e.ch <- x
	}
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
	<-e.ch
}

// Result is called to get the extracted metadata
func (e *ExifExtractor) Result() Metadata {
	m := Metadata{}
	x := <-e.ch
	switch x := x.(type) {
	case *exif.Exif:
		if dt, err := x.DateTime(); err == nil {
			m["datetime"] = dt
		}
		// TODO add other metadata
	}
	return m
}
