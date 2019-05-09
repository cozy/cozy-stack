package vfs

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"math"
	"time"

	// Packages image/... are not used explicitly in the code below,
	// but are imported for their initialization side-effects
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	// Same for image/webp
	_ "golang.org/x/image/webp"

	"github.com/cozy/goexif2/exif"
	"github.com/dhowden/tag"
)

// MetadataExtractorVersion is the version number of the metadata extractor.
// It will be used later to know which files can be re-examined to get more
// metadata when the extractor is improved.
const MetadataExtractorVersion = 2

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
		e = NewExifExtractor(doc.CreatedAt, true)
	case "image/heic", "image/heif":
		e = NewExifExtractor(doc.CreatedAt, false)
	case "image/png", "image/gif":
		e = NewImageExtractor(doc.CreatedAt)
	case "audio/mp3", "audio/mpeg", "audio/ogg", "audio/x-m4a", "audio/flac":
		e = NewAudioExtractor()
	}
	if e != nil {
		return &e
	}
	return nil
}

// ImageExtractor is used to extract width/height from images
type ImageExtractor struct {
	w         *io.PipeWriter
	r         *io.PipeReader
	ch        chan interface{}
	createdAt time.Time
}

// NewImageExtractor returns an extractor for images
func NewImageExtractor(createdAt time.Time) *ImageExtractor {
	e := &ImageExtractor{createdAt: createdAt}
	e.r, e.w = io.Pipe()
	e.ch = make(chan interface{})
	go e.Start()
	return e
}

// Start is used in a goroutine to start the metadata extraction
func (e *ImageExtractor) Start() {
	var cfg image.Config
	var err error
	defer func() {
		r := recover()
		if errc := e.r.Close(); err == nil {
			err = errc
		}
		if r != nil {
			e.ch <- fmt.Errorf("metadata: recovered from image decoding: %s", r)
		} else if err != nil {
			e.ch <- err
		} else {
			e.ch <- cfg
		}
	}()
	cfg, _, err = image.DecodeConfig(e.r)
}

// Write is called to push some bytes to the extractor
func (e *ImageExtractor) Write(p []byte) (n int, err error) {
	return e.w.Write(p)
}

// Close is called when all the bytes has been pushed, to finalize the extraction
func (e *ImageExtractor) Close() error {
	err := e.w.Close()
	if err != nil {
		<-e.ch
	}
	return err
}

// Abort is called when the extractor can be discarded
func (e *ImageExtractor) Abort(err error) {
	_ = e.w.CloseWithError(err)
	<-e.ch
}

// Result is called to get the extracted metadata
func (e *ImageExtractor) Result() Metadata {
	m := NewMetadata()
	m["datetime"] = e.createdAt
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
func NewExifExtractor(createdAt time.Time, withImageExtractor bool) *ExifExtractor {
	e := &ExifExtractor{}
	if withImageExtractor {
		e.im = NewImageExtractor(createdAt)
	}
	e.r, e.w = io.Pipe()
	e.ch = make(chan interface{})
	go e.Start()
	return e
}

// Start is used in a goroutine to start the metadata extraction
func (e *ExifExtractor) Start() {
	var x *exif.Exif
	var err error
	defer func() {
		r := recover()
		if errc := e.r.Close(); err == nil {
			err = errc
		}
		if r != nil {
			e.ch <- fmt.Errorf("metadata: recovered from exif extracting: %s", r)
		} else if err != nil {
			e.ch <- err
		} else {
			e.ch <- x
		}
	}()
	x, err = exif.Decode(e.r)
}

// Write is called to push some bytes to the extractor
func (e *ExifExtractor) Write(p []byte) (n int, err error) {
	if e.im != nil {
		_, _ = e.im.Write(p)
	}
	return e.w.Write(p)
}

// Close is called when all the bytes has been pushed, to finalize the extraction
func (e *ExifExtractor) Close() error {
	if e.im != nil {
		e.im.Close()
	}
	return e.w.Close()
}

// Abort is called when the extractor can be discarded
func (e *ExifExtractor) Abort(err error) {
	if e.im != nil {
		e.im.Abort(err)
	}
	_ = e.w.CloseWithError(err)
	<-e.ch
}

// Result is called to get the extracted metadata
func (e *ExifExtractor) Result() Metadata {
	var m Metadata
	if e.im != nil {
		m = e.im.Result()
	} else {
		m = NewMetadata()
	}
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
			if !math.IsNaN(lat) && !math.IsNaN(long) {
				m["gps"] = map[string]float64{
					"lat":  lat,
					"long": long,
				}
			}
		}
		if _, ok := m["width"]; !ok {
			if xDimension, err := x.Get("PixelXDimension"); err == nil {
				if width, err := xDimension.Int(0); err == nil {
					m["width"] = width
				}
			}
		}
		if _, ok := m["height"]; !ok {
			if yDimension, err := x.Get("PixelYDimension"); err == nil {
				if height, err := yDimension.Int(0); err == nil {
					m["height"] = height
				}
			}
		}
		if o, err := x.Get("Orientation"); err == nil {
			if orientation, err := o.Int(0); err == nil {
				m["orientation"] = orientation
			}
		}
	}
	return m
}

// AudioExtractor is used to extract album/artist/etc. from audio
type AudioExtractor struct {
	w  *io.PipeWriter
	r  *io.PipeReader
	ch chan interface{}
}

// NewAudioExtractor returns an extractor for audio
func NewAudioExtractor() *AudioExtractor {
	e := &AudioExtractor{}
	e.r, e.w = io.Pipe()
	e.ch = make(chan interface{})
	go e.Start()
	return e
}

// Start is used in a goroutine to start the metadata extraction
func (e *AudioExtractor) Start() {
	var tags tag.Metadata
	var buf []byte
	var err error
	buf, err = ioutil.ReadAll(e.r)
	if err != nil {
		e.r.Close()
		e.ch <- err
		return
	}
	defer func() {
		r := recover()
		if errc := e.r.Close(); err == nil {
			err = errc
		}
		if r != nil {
			e.ch <- fmt.Errorf("metadata: recovered from audio extracting: %s", r)
		} else if err != nil {
			e.ch <- err
		} else {
			e.ch <- tags
		}
	}()
	tags, err = tag.ReadFrom(bytes.NewReader(buf))
}

// Write is called to push some bytes to the extractor
func (e *AudioExtractor) Write(p []byte) (n int, err error) {
	return e.w.Write(p)
}

// Close is called when all the bytes has been pushed, to finalize the extraction
func (e *AudioExtractor) Close() error {
	return e.w.Close()
}

// Abort is called when the extractor can be discarded
func (e *AudioExtractor) Abort(err error) {
	_ = e.w.CloseWithError(err)
	<-e.ch
}

// Result is called to get the extracted metadata
func (e *AudioExtractor) Result() Metadata {
	m := NewMetadata()
	tags := <-e.ch
	switch tags := tags.(type) {
	case tag.Metadata:
		if album := tags.Album(); album != "" {
			m["album"] = album
		}
		if artist := tags.Artist(); artist != "" {
			m["artist"] = artist
		}
		if composer := tags.Composer(); composer != "" {
			m["composer"] = composer
		}
		if genre := tags.Genre(); genre != "" {
			m["genre"] = genre
		}
		if title := tags.Title(); title != "" {
			m["title"] = title
		}
		if year := tags.Year(); year != 0 {
			m["year"] = year
		}
		if track, _ := tags.Track(); track != 0 {
			m["track"] = track
		}
	}
	return m
}
