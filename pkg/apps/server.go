package apps

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/base64"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/cozy/afero"
	"github.com/cozy/cozy-stack/pkg/magic"
	webutils "github.com/cozy/cozy-stack/web/utils"
	"github.com/cozy/swift"
)

// FileServer interface defines a way to access and serve the application's
// data files.
type FileServer interface {
	Open(slug, version, file string) (io.ReadCloser, error)
	ServeFileContent(w http.ResponseWriter, req *http.Request,
		slug, version, file string) error
}

type swiftServer struct {
	c         *swift.Connection
	container string
}

type aferoServer struct {
	mkPath func(slug, version, file string) string
	fs     afero.Fs
}

type gzipReadCloser struct {
	gr *gzip.Reader
	cl io.Closer
}

// The Close method of gzip.Reader does not closes the underlying reader. This
// little wrapper does the closing.
func newGzipReadCloser(r io.ReadCloser) (io.ReadCloser, error) {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	return gzipReadCloser{gr: gr, cl: r}, nil
}

func (g gzipReadCloser) Read(b []byte) (int, error) {
	return g.gr.Read(b)
}

func (g gzipReadCloser) Close() error {
	err1 := g.gr.Close()
	err2 := g.cl.Close()
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
}

// NewSwiftFileServer returns provides the apps.FileServer implementation
// using the swift backend as file server.
func NewSwiftFileServer(conn *swift.Connection, appsType AppType) FileServer {
	return &swiftServer{
		c:         conn,
		container: containerName(appsType),
	}
}

func (s *swiftServer) Open(slug, version, file string) (io.ReadCloser, error) {
	objName := s.makeObjectName(slug, version, file)
	f, h, err := s.c.ObjectOpen(s.container, objName, false, nil)
	if err != nil {
		return nil, wrapSwiftErr(err)
	}
	o := h.ObjectMetadata()
	if contentEncoding := o["content-encoding"]; contentEncoding == "gzip" {
		return newGzipReadCloser(f)
	}
	return f, nil
}

func (s *swiftServer) ServeFileContent(w http.ResponseWriter, req *http.Request, slug, version, file string) error {
	objName := s.makeObjectName(slug, version, file)
	f, h, err := s.c.ObjectOpen(s.container, objName, false, nil)
	if err != nil {
		return wrapSwiftErr(err)
	}
	defer f.Close()

	if checkETag := req.Header.Get("Cache-Control") == ""; checkETag {
		var eTag string
		if eTag = h["Etag"]; eTag != "" && len(eTag) > 10 {
			eTag = eTag[:10]
		}
		if webutils.CheckPreconditions(w, req, eTag) {
			return nil
		}
	}

	var r io.Reader = f
	contentLength := h["Content-Length"]
	contentType := h["Content-Type"]
	o := h.ObjectMetadata()
	if contentEncoding := o["content-encoding"]; contentEncoding == "gzip" {
		if acceptGzipEncoding(req) {
			w.Header().Set("Content-Encoding", "gzip")
		} else {
			contentLength = o["original-content-length"]
			var gr *gzip.Reader
			gr, err = gzip.NewReader(f)
			if err != nil {
				return err
			}
			defer gr.Close()
			r = gr
		}
	}

	ext := path.Ext(file)
	if contentType == "" {
		contentType = magic.MIMETypeByExtension(ext)
	}
	if contentType == "text/html" {
		contentType = "text/html; charset=utf-8"
	} else if contentType == "text/xml" && ext == ".svg" {
		// override for files with text/xml content because of leading <?xml tag
		contentType = "image/svg+xml"
	}

	size, _ := strconv.ParseInt(contentLength, 10, 64)
	webutils.ServeContent(w, req, contentType, size, r)
	return nil
}

func (s *swiftServer) makeObjectName(slug, version, file string) string {
	return path.Join(slug, version, file)
}

// NewAferoFileServer returns a simple wrapper of the afero.Fs interface that
// provides the apps.FileServer interface.
//
// You can provide a makePath method to define how the file name should be
// created from the application's slug, version and file name. If not provided,
// the standard VFS concatenation (starting with vfs.WebappsDirName) is used.
func NewAferoFileServer(fs afero.Fs, makePath func(slug, version, file string) string) FileServer {
	if makePath == nil {
		makePath = defaultMakePath
	}
	return &aferoServer{
		mkPath: makePath,
		fs:     fs,
	}
}

func (s *aferoServer) Open(slug, version, file string) (io.ReadCloser, error) {
	isGzipped := true
	filepath := s.mkPath(slug, version, file)
	f, err := s.open(filepath + ".gz")
	if os.IsNotExist(err) {
		isGzipped = false
		f, err = s.open(filepath)
	}
	if err != nil {
		return nil, err
	}
	if isGzipped {
		return newGzipReadCloser(f)
	}
	return f, nil
}
func (s *aferoServer) open(filepath string) (io.ReadCloser, error) {
	return s.fs.Open(filepath)
}

func (s *aferoServer) ServeFileContent(w http.ResponseWriter, req *http.Request, slug, version, file string) error {
	filepath := s.mkPath(slug, version, file)
	return s.serveFileContent(w, req, filepath)
}
func (s *aferoServer) serveFileContent(w http.ResponseWriter, req *http.Request, filepath string) error {
	isGzipped := true
	rc, err := s.fs.Open(filepath + ".gz")
	if os.IsNotExist(err) {
		isGzipped = false
		rc, err = s.fs.Open(filepath)
	}
	if err != nil {
		return err
	}
	defer rc.Close()

	var content io.Reader
	var size int64
	if checkEtag := req.Header.Get("Cache-Control") == ""; checkEtag {
		var b []byte
		h := md5.New()
		r := io.TeeReader(rc, h)
		b, err = ioutil.ReadAll(r)
		if err != nil {
			return err
		}
		eTag := base64.StdEncoding.EncodeToString(h.Sum(nil))
		if webutils.CheckPreconditions(w, req, eTag) {
			return nil
		}
		size = int64(len(b))
		content = bytes.NewReader(b)
	} else {
		size, err = rc.Seek(0, io.SeekEnd)
		if err != nil {
			return err
		}
		_, err = rc.Seek(0, io.SeekStart)
		if err != nil {
			return err
		}
		content = rc
	}

	if isGzipped {
		if acceptGzipEncoding(req) {
			w.Header().Set("Content-Encoding", "gzip")
		} else {
			var gr *gzip.Reader
			gr, err = gzip.NewReader(content)
			if err != nil {
				return err
			}
			defer gr.Close()
			content = gr
		}
	}

	contentType := magic.MIMETypeByExtension(path.Ext(filepath))
	if contentType == "text/html" {
		contentType = "text/html; charset=utf-8"
	}
	webutils.ServeContent(w, req, contentType, size, content)
	return nil
}

func defaultMakePath(slug, version, file string) string {
	basepath := path.Join("/", slug, version)
	filepath := path.Join("/", file)
	return path.Join(basepath, filepath)
}

func acceptGzipEncoding(req *http.Request) bool {
	return strings.Contains(req.Header.Get("Accept-Encoding"), "gzip")
}

func containerName(appsType AppType) string {
	switch appsType {
	case Webapp:
		return "apps-web"
	case Konnector:
		return "apps-konnectors"
	}
	panic("Unknown AppType")
}

func wrapSwiftErr(err error) error {
	if err == swift.ObjectNotFound || err == swift.ContainerNotFound {
		return os.ErrNotExist
	}
	return err
}
