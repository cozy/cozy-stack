package apps

import (
	"io"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/ncw/swift"
	"github.com/spf13/afero"
)

// FileServer interface defines a way to access and serve the application's
// data files.
type FileServer interface {
	Stat(slug, version, file string) (os.FileInfo, error)
	Open(slug, version, file string) (File, error)
	ServeFileContent(w http.ResponseWriter, req *http.Request,
		modtime time.Time, slug, version, file string) error
}

type File interface {
	io.ReadCloser
	io.Seeker
}

type swiftServer struct {
	c         *swift.Connection
	container string
}

type aferoServer struct {
	mkPath func(slug, version, file string) string
	fs     afero.Fs
}

type fileStat struct {
	name    string
	size    int64
	modtime time.Time
	isDir   bool
}

// NewSwiftFileServer returns provides the apps.FileServer implementation
// using the swift backend as file server.
func NewSwiftFileServer(conn *swift.Connection) FileServer {
	return &swiftServer{
		c: conn,
	}
}

func (s *swiftServer) Stat(slug, version, file string) (os.FileInfo, error) {
	objName := s.makeObjectName(slug, version, file)
	o, _, err := s.c.Object(s.container, objName)
	if err != nil {
		return nil, wrapSwiftErr(err)
	}
	return &fileStat{
		name:    objName,
		size:    o.Bytes,
		modtime: o.LastModified,
		isDir:   false,
	}, nil
}

func (s *swiftServer) Open(slug, version, file string) (File, error) {
	objName := s.makeObjectName(slug, version, file)
	f, _, err := s.c.ObjectOpen(s.container, objName, false, nil)
	if err != nil {
		return nil, wrapSwiftErr(err)
	}
	return f, nil
}

func (s *swiftServer) ServeFileContent(w http.ResponseWriter, req *http.Request, modtime time.Time, slug, version, file string) error {
	objName := s.makeObjectName(slug, version, file)
	f, o, err := s.c.ObjectOpen(s.container, objName, false, nil)
	if err != nil {
		return wrapSwiftErr(err)
	}
	defer f.Close()
	lastModified, _ := time.Parse(http.TimeFormat, o["Last-Modified"])
	w.Header().Set("Etag", o["Etag"])
	http.ServeContent(w, req, objName, lastModified, f)
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

func (s *aferoServer) Stat(slug, version, file string) (os.FileInfo, error) {
	return s.fs.Stat(s.mkPath(slug, version, file))
}

func (s *aferoServer) Open(slug, version, file string) (File, error) {
	return s.fs.Open(s.mkPath(slug, version, file))
}

func (s *aferoServer) ServeFileContent(w http.ResponseWriter, req *http.Request, modtime time.Time, slug, version, file string) error {
	filepath := s.mkPath(slug, version, file)
	r, err := s.fs.Open(filepath)
	if err != nil {
		return err
	}
	defer r.Close()
	http.ServeContent(w, req, filepath, modtime, r)
	return nil
}

func defaultMakePath(slug, version, file string) string {
	return path.Join("/", slug, version, file)
}

func (f *fileStat) IsDir() bool        { return f.isDir }
func (f *fileStat) ModTime() time.Time { return f.modtime }
func (f *fileStat) Mode() os.FileMode  { return 0 }
func (f *fileStat) Name() string       { return f.name }
func (f *fileStat) Size() int64        { return f.size }
func (f *fileStat) Sys() interface{}   { return nil }
