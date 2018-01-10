package apps

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/pkg/magic"
	web_utils "github.com/cozy/cozy-stack/web/utils"
	"github.com/cozy/swift"
	"github.com/spf13/afero"
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
		return gzip.NewReader(f)
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

	if checkETag := req.Header.Get("Cache-Control") != ""; checkETag {
		etag := fmt.Sprintf(`"%s"`, h["Etag"][:10])
		if web_utils.CheckPreconditions(w, req, etag) {
			return nil
		}
		w.Header().Set("Etag", etag)
	}

	var r io.Reader = f
	contentLength := h["Content-Length"]
	contentType := h["Content-Type"]
	o := h.ObjectMetadata()
	if contentEncoding := o["content-encoding"]; contentEncoding == "gzip" {
		if strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
		} else {
			contentLength = o["original-content-length"]
			r, err = gzip.NewReader(f)
			if err != nil {
				return err
			}
		}
	}

	if contentType == "" {
		contentType = magic.MIMETypeByExtension(path.Ext(file))
	}

	size, _ := strconv.ParseInt(contentLength, 10, 64)
	web_utils.ServeContent(w, req, contentType, size, r)
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
	if os.IsNotExist(err) {
		return s.open(retroCompatMakePath(slug, version, file))
	}
	if err != nil {
		return nil, err
	}
	if isGzipped {
		return gzip.NewReader(f)
	}
	return f, nil
}
func (s *aferoServer) open(filepath string) (io.ReadCloser, error) {
	return s.fs.Open(filepath)
}

func (s *aferoServer) ServeFileContent(w http.ResponseWriter, req *http.Request, slug, version, file string) error {
	filepath := s.mkPath(slug, version, file)
	err := s.serveFileContent(w, req, filepath)
	if os.IsNotExist(err) {
		return s.serveFileContent(w, req, retroCompatMakePath(slug, version, file))
	}
	return err
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
	if checkEtag := req.Header.Get("Cache-Control") != ""; checkEtag {
		var b []byte
		h := md5.New()
		r := io.TeeReader(rc, h)
		b, err = ioutil.ReadAll(r)
		if err != nil {
			return err
		}
		etag := fmt.Sprintf(`"%s"`, hex.EncodeToString(h.Sum(nil)))
		if web_utils.CheckPreconditions(w, req, etag) {
			return nil
		}
		w.Header().Set("Etag", etag)
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
		w.Header().Set("Content-Encoding", "gzip")
	}

	contentType := magic.MIMETypeByExtension(path.Ext(filepath))
	web_utils.ServeContent(w, req, contentType, size, content)
	return nil
}

func defaultMakePath(slug, version, file string) string {
	basepath := path.Join("/", slug, version)
	filepath := path.Join("/", file)
	return path.Join(basepath, filepath)
}

// FIXME: retro-compatibility code to serve application that were not installed
// in a versioned directory.
func retroCompatMakePath(slug, version, file string) string {
	return path.Join("/", slug, file)
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
