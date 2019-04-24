package appfs

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/cozy/afero"
	"github.com/cozy/cozy-stack/pkg/consts"
	web_utils "github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/swift"
)

// FileServer interface defines a way to access and serve the application's
// data files.
type FileServer interface {
	Open(slug, version, shasum, file string) (io.ReadCloser, error)
	FilesList(slug, version, shasum string) ([]string, error)
	ServeFileContent(w http.ResponseWriter, req *http.Request,
		slug, version, shasum, file string) error
}

type swiftServer struct {
	c         *swift.Connection
	container string
}

type aferoServer struct {
	mkPath func(slug, version, shasum, file string) string
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
func NewSwiftFileServer(conn *swift.Connection, appsType consts.AppType) FileServer {
	return &swiftServer{
		c:         conn,
		container: containerName(appsType),
	}
}

func (s *swiftServer) Open(slug, version, shasum, file string) (io.ReadCloser, error) {
	objName := s.makeObjectName(slug, version, shasum, file)
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

func (s *swiftServer) ServeFileContent(w http.ResponseWriter, req *http.Request, slug, version, shasum, file string) error {
	objName := s.makeObjectName(slug, version, shasum, file)
	f, h, err := s.c.ObjectOpen(s.container, objName, false, nil)
	if err != nil {
		return wrapSwiftErr(err)
	}
	defer f.Close()

	if checkETag := req.Header.Get("Cache-Control") == ""; checkETag {
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
		contentType = mime.TypeByExtension(ext)
	}
	if contentType == "text/xml" && ext == ".svg" {
		// override for files with text/xml content because of leading <?xml tag
		contentType = "image/svg+xml"
	}

	size, _ := strconv.ParseInt(contentLength, 10, 64)
	web_utils.ServeContent(w, req, contentType, size, r)
	return nil
}

func (s *swiftServer) makeObjectName(slug, version, shasum, file string) string {
	basepath := path.Join(slug, version)
	if shasum != "" {
		basepath += "-" + shasum
	}
	return path.Join(basepath, file)
}

func (s *swiftServer) FilesList(slug, version, shasum string) ([]string, error) {
	prefix := s.makeObjectName(slug, version, shasum, "") + "/"
	names, err := s.c.ObjectNamesAll(s.container, &swift.ObjectsOpts{
		Prefix: prefix,
	})
	if err != nil {
		return nil, err
	}
	filtered := names[:0]
	for _, n := range names {
		n = strings.TrimPrefix(n, prefix)
		if n != "" {
			filtered = append(filtered, n)
		}
	}
	return filtered, nil
}

// NewAferoFileServer returns a simple wrapper of the afero.Fs interface that
// provides the apps.FileServer interface.
//
// You can provide a makePath method to define how the file name should be
// created from the application's slug, version and file name. If not provided,
// the standard VFS concatenation (starting with vfs.WebappsDirName) is used.
func NewAferoFileServer(fs afero.Fs, makePath func(slug, version, shasum, file string) string) FileServer {
	if makePath == nil {
		makePath = defaultMakePath
	}
	return &aferoServer{
		mkPath: makePath,
		fs:     fs,
	}
}

func (s *aferoServer) Open(slug, version, shasum, file string) (io.ReadCloser, error) {
	isGzipped := true
	filepath := s.mkPath(slug, version, shasum, file)
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

func (s *aferoServer) ServeFileContent(w http.ResponseWriter, req *http.Request, slug, version, shasum, file string) error {
	filepath := s.mkPath(slug, version, shasum, file)
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
		if acceptGzipEncoding(req) {
			w.Header().Set("Content-Encoding", "gzip")
		} else {
			var gr *gzip.Reader
			var b []byte
			gr, err = gzip.NewReader(content)
			if err != nil {
				return err
			}
			defer gr.Close()
			b, err = ioutil.ReadAll(gr)
			if err != nil {
				return err
			}
			size = int64(len(b))
			content = bytes.NewReader(b)
		}
	}

	contentType := mime.TypeByExtension(path.Ext(filepath))
	web_utils.ServeContent(w, req, contentType, size, content)
	return nil
}

func (s *aferoServer) FilesList(slug, version, shasum string) ([]string, error) {
	var names []string
	rootPath := s.mkPath(slug, version, shasum, "")
	err := afero.Walk(s.fs, rootPath, func(path string, infos os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !infos.IsDir() {
			name := strings.TrimPrefix(path, rootPath)
			name = strings.TrimSuffix(name, ".gz")
			names = append(names, name)
		}
		return nil
	})
	return names, err
}

func defaultMakePath(slug, version, shasum, file string) string {
	basepath := path.Join("/", slug, version)
	if shasum != "" {
		basepath += "-" + shasum
	}
	filepath := path.Join("/", file)
	return path.Join(basepath, filepath)
}

func acceptGzipEncoding(req *http.Request) bool {
	return strings.Contains(req.Header.Get("Accept-Encoding"), "gzip")
}

func containerName(appsType consts.AppType) string {
	switch appsType {
	case consts.WebappType:
		return "apps-web"
	case consts.KonnectorType:
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
