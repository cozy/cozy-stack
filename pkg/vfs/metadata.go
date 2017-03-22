package vfs

import (
	"image"
	"io"

	// Packages image/... are not used explicitly in the code below,
	// but are imported for its initialization side-effect
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	// Same for image/webp
	_ "golang.org/x/image/webp"

	"github.com/xor-gate/goexif2/exif"
)

// MetadataExtractorVersion is the version number of the metadata extractor.
// It will be used later to know which files can be re-examined to get more
// metadata when the extractor is improved.
const MetadataExtractorVersion = 1

// Metadata is a list of metadata specific to each mimetype:
// id3 for music, exif for jpegs, etc.
type Metadata map[string]interface{}

// NewMetadata returns a new metadata object, with the version field set
func NewMetadata() Metadata {
	m := Metadata{}
	m["extractor_version"] = MetadataExtractorVersion
	return m
}

// MetaExtractor is an interface for extracting metadata from a file
type MetaExtractor interface {
	io.WriteCloser
	Abort(error)
	Result() Metadata
}

// NewMetaExtractor returns an extractor for metadata if the mime type has one,
// or null else
func NewMetaExtractor(doc *FileDoc) *MetaExtractor {
	var e MetaExtractor
	switch doc.Mime {
	case "image/jpeg":
		e = NewExifExtractor()
	case "image/png", "image/gif":
		e = NewImageExtractor()
	}
	if e != nil {
		return &e
	}
	return nil
}

// ImageExtractor is used to extract width/height from images
type ImageExtractor struct {
	w  *io.PipeWriter
	r  *io.PipeReader
	ch chan interface{}
}

// NewImageExtractor returns an extractor for images
func NewImageExtractor() *ImageExtractor {
	e := &ImageExtractor{}
	e.r, e.w = io.Pipe()
	e.ch = make(chan interface{})
	go e.Start()
	return e
}

// Start is used in a goroutine to start the metadata extraction
func (e *ImageExtractor) Start() {
	cfg, _, err := image.DecodeConfig(e.r)
	e.r.Close()
	if err != nil {
		e.ch <- err
	} else {
		e.ch <- cfg
	}
}

// Write is called to push some bytes to the extractor
func (e *ImageExtractor) Write(p []byte) (n int, err error) {
	return e.w.Write(p)
}

// Close is called when all the bytes has been pushed, to finalize the extraction
func (e *ImageExtractor) Close() error {
	return e.w.Close()
}

// Abort is called when the extractor can be discarded
func (e *ImageExtractor) Abort(err error) {
	e.w.CloseWithError(err)
	<-e.ch
}

// Result is called to get the extracted metadata
func (e *ImageExtractor) Result() Metadata {
	m := NewMetadata()
	cfg := <-e.ch
	switch cfg := cfg.(type) {
	case image.Config:
		m["width"] = cfg.Width
		m["height"] = cfg.Height
	}
	return m
}

// ExifExtractor is used to extract EXIF metadata from jpegs
type ExifExtractor struct {
	w  *io.PipeWriter
	r  *io.PipeReader
	im *ImageExtractor
	ch chan interface{}
}

// NewExifExtractor returns an extractor for EXIF metadata
func NewExifExtractor() *ExifExtractor {
	e := &ExifExtractor{}
	e.im = NewImageExtractor()
	e.r, e.w = io.Pipe()
	e.ch = make(chan interface{})
	go e.Start()
	return e
}

// Start is used in a goroutine to start the metadata extraction
func (e *ExifExtractor) Start() {
	x, err := exif.Decode(e.r)
	e.r.Close()
	if err != nil {
		e.ch <- err
	} else {
		e.ch <- x
	}
}

// Write is called to push some bytes to the extractor
func (e *ExifExtractor) Write(p []byte) (n int, err error) {
	e.im.Write(p)
	return e.w.Write(p)
}

// Close is called when all the bytes has been pushed, to finalize the extraction
func (e *ExifExtractor) Close() error {
	e.im.Close()
	return e.w.Close()
}

// Abort is called when the extractor can be discarded
func (e *ExifExtractor) Abort(err error) {
	e.im.Abort(err)
	e.w.CloseWithError(err)
	<-e.ch
}

// Result is called to get the extracted metadata
func (e *ExifExtractor) Result() Metadata {
	m := e.im.Result()
	x := <-e.ch
	switch x := x.(type) {
	case *exif.Exif:
		if dt, err := x.DateTime(); err == nil {
			m["datetime"] = dt
		}
		if flash, err := x.Flash(); err == nil {
			m["flash"] = flash
		}
		if lat, long, err := x.LatLong(); err == nil {
			m["gps"] = map[string]float64{
				"lat":  lat,
				"long": long,
			}
		}
	}
	return m
}
