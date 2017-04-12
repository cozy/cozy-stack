package apps

import (
	"errors"
	"io"
	"os"
	"path"

	"github.com/ncw/swift"
	"github.com/spf13/afero"
)

type Copier interface {
	Start(slug, version string) (exists bool, err error)
	Copy(name string, src io.Reader) error
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

func (f *swiftCopier) Copy(name string, src io.Reader) (err error) {
	if !f.started {
		return errors.New("copier should call Start() before Copy()")
	}
	defer func() {
		if err != nil {
			f.c.ObjectDelete(f.container, f.rootObj) // #nosec
		}
	}()
	objName := path.Join(f.rootObj, name)
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

func (f *aferoCopier) Copy(name string, src io.Reader) (err error) {
	if !f.started {
		return errors.New("copier should call Start() before Copy()")
	}
	fullpath := path.Join(f.appDir, name)
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

func wrapSwiftErr(err error) error {
	if err == nil {
		return nil
	}
	if err == swift.ObjectNotFound {
		return os.ErrNotExist
	}
	return nil
}
