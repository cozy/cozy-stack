package apps

import (
	"archive/tar"
	"io"
	"os"
	"path"
	"time"

	"github.com/ncw/swift"
	"github.com/spf13/afero"
)

// Copier is an interface defining a common set of functions for the installer
// to copy the application into an unknown storage.
type Copier interface {
	Start(slug, version string) (exists bool, err error)
	Copy(stat os.FileInfo, src io.Reader) error
	Close() error
}

type swiftCopier struct {
	c         *swift.Connection
	rootObj   string
	container string
	started   bool
}

type aferoCopier struct {
	fs      afero.Fs
	appDir  string
	started bool
}

// NewSwiftCopier defines a Copier storing data into a swift container.
func NewSwiftCopier(conn *swift.Connection, container string) (Copier, error) {
	if container[0] == '/' {
		container = container[1:]
	}
	if err := conn.ContainerCreate(container, nil); err != nil {
		return nil, err
	}
	return &swiftCopier{c: conn, container: container}, nil
}

func (f *swiftCopier) Start(slug, version string) (bool, error) {
	f.rootObj = path.Join(slug, version)
	_, _, err := f.c.Object(f.container, f.rootObj)
	if err == nil {
		return true, nil
	}
	o, err := f.c.ObjectCreate(f.container, f.rootObj, false, "", "", nil)
	if err != nil {
		return false, err
	}
	err = o.Close()
	f.started = err == nil
	return false, err
}

func (f *swiftCopier) Copy(stat os.FileInfo, src io.Reader) (err error) {
	if !f.started {
		panic("copier should call Start() before Copy()")
	}
	defer func() {
		if err != nil {
			f.c.ObjectDelete(f.container, f.rootObj) // #nosec
		}
	}()
	objName := path.Join(f.rootObj, stat.Name())
	file, err := f.c.ObjectCreate(f.container, objName, false, "", "", nil)
	if err != nil {
		return err
	}
	defer func() {
		if errc := file.Close(); errc != nil {
			err = errc
		}
	}()
	_, err = io.Copy(file, src)
	return err
}

func (f *swiftCopier) Close() error {
	return nil
}

// NewAferoCopier defines a copier using an afero.Fs filesystem to store the
// application data.
func NewAferoCopier(fs afero.Fs) Copier {
	return &aferoCopier{fs: fs}
}

func (f *aferoCopier) Start(slug, version string) (bool, error) {
	f.appDir = path.Join("/", slug, version)
	exists, err := afero.DirExists(f.fs, f.appDir)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}
	err = f.fs.MkdirAll(f.appDir, 0755)
	f.started = err == nil
	return false, err
}

func (f *aferoCopier) Copy(stat os.FileInfo, src io.Reader) (err error) {
	if !f.started {
		panic("copier should call Start() before Copy()")
	}
	fullpath := path.Join(f.appDir, stat.Name())
	dir := path.Dir(fullpath)
	if err = f.fs.MkdirAll(dir, 0755); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			f.fs.RemoveAll(f.appDir) // #nosec
		}
	}()
	dst, err := f.fs.Create(fullpath)
	if err != nil {
		return err
	}
	defer func() {
		if errc := dst.Close(); errc != nil {
			err = errc
		}
	}()
	_, err = io.Copy(dst, src)
	return err
}

func (f *aferoCopier) Close() error {
	return nil
}

type tarCopier struct {
	src  Copier
	name string

	tmp afero.File
	fs  afero.Fs
	tw  *tar.Writer
}

// newTarCopier defines a Copier that will copy all the files into an tar
// archive before copying that archive into the specified source Copier.
func newTarCopier(src Copier, name string) Copier {
	return &tarCopier{
		src:  src,
		name: name,
	}
}

func (t *tarCopier) Start(slug, version string) (bool, error) {
	if exists, err := t.src.Start(slug, version); err != nil || exists {
		return exists, err
	}
	fs := afero.NewOsFs()
	tmp, err := afero.TempFile(fs, "", "konnector-")
	if err != nil {
		return false, err
	}
	tw := tar.NewWriter(tmp)
	t.tmp = tmp
	t.fs = fs
	t.tw = tw
	return false, nil
}

func (t *tarCopier) Copy(stat os.FileInfo, src io.Reader) error {
	hdr := &tar.Header{
		Name: stat.Name(),
		Mode: int64(stat.Mode()),
		Size: stat.Size(),
	}
	if err := t.tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := io.Copy(t.tw, src)
	return err
}

func (t *tarCopier) Close() (err error) {
	defer func() {
		if t.tmp != nil {
			t.fs.Remove(t.tmp.Name()) // #nosec
		}
		if errc := t.src.Close(); errc != nil && err == nil {
			err = errc
		}
	}()
	if t.tw == nil || t.tmp == nil {
		return nil
	}
	if err = t.tw.Flush(); err != nil {
		return err
	}
	if _, err = t.tmp.Seek(0, 0); err != nil {
		return err
	}
	return t.src.Copy(&fileInfo{name: KonnectorArchiveName}, t.tmp)
}

func wrapSwiftErr(err error) error {
	if err == nil {
		return nil
	}
	if err == swift.ObjectNotFound {
		return os.ErrNotExist
	}
	return nil
}

type fileInfo struct {
	name string
	size int64
	mode os.FileMode
	time time.Time
}

func (f *fileInfo) Name() string       { return f.name }
func (f *fileInfo) Size() int64        { return f.size }
func (f *fileInfo) Mode() os.FileMode  { return f.mode }
func (f *fileInfo) ModTime() time.Time { return f.time }
func (f *fileInfo) IsDir() bool        { return false }
func (f *fileInfo) Sys() interface{}   { return nil }
