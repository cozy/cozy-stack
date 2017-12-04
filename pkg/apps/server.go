package apps

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/cozy/swift"
	"github.com/spf13/afero"
)

var unixZeroEpoch = time.Time{}

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
	f, _, err := s.c.ObjectOpen(s.container, objName, false, nil)
	if err != nil {
		return nil, wrapSwiftErr(err)
	}
	return f, nil
}

func (s *swiftServer) ServeFileContent(w http.ResponseWriter, req *http.Request, slug, version, file string) error {
	objName := s.makeObjectName(slug, version, file)
	f, o, err := s.c.ObjectOpen(s.container, objName, false, nil)
	if err != nil {
		return wrapSwiftErr(err)
	}
	defer f.Close()

	w.Header().Set("Cache-Control", "no-cache")
	if req.Header.Get("Range") == "" {
		w.Header().Set("Etag", fmt.Sprintf(`"%s"`, o["Etag"]))
	}

	http.ServeContent(w, req, objName, unixZeroEpoch, f)
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
	filepath := s.mkPath(slug, version, file)
	f, err := s.open(filepath)
	if os.IsNotExist(err) {
		return s.open(retroCompatMakePath(slug, version, file))
	}
	return f, err
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
	rc, err := s.fs.Open(filepath)
	if err != nil {
		return err
	}
	defer rc.Close()

	h := md5.New()
	r := io.TeeReader(rc, h)
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	if req.Header.Get("Range") == "" {
		w.Header().Set("Etag", fmt.Sprintf(`"%s"`, hex.EncodeToString(h.Sum(nil))))
	}

	http.ServeContent(w, req, filepath, unixZeroEpoch, bytes.NewReader(b))
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
