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
	Copy(slug, version, name string, src io.Reader) error
	Close() error
}

type swiftCopier struct {
	c         *swift.Connection
	container string
	started   bool
}

type aferoCopier struct {
	fs      afero.Fs
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
	objName := path.Join(slug, version)
	_, _, err := f.c.Object(f.container, objName)
	if err == nil {
		return true, nil
	}
	o, err := f.c.ObjectCreate(f.container, objName, false, "", "", nil)
	if err != nil {
		return false, err
	}
	err = o.Close()
	f.started = err == nil
	return false, err
}

func (f *swiftCopier) Copy(slug, version, name string, src io.Reader) (err error) {
	if !f.started {
		return errors.New("copier should call Start() before Copy()")
	}
	defer func() {
		if err != nil {
			// TODO: retries on this important delete
			f.c.ObjectDelete(f.container, path.Join(slug, version)) // #nosec
		}
	}()
	objName := path.Join(slug, version, name)
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

func NewAferoCopier(fs afero.Fs) Copier {
	return &aferoCopier{fs: fs}
}

func (f *aferoCopier) Start(slug, version string) (bool, error) {
	appDir := path.Join("/", slug, version)
	exists, err := afero.DirExists(f.fs, appDir)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}
	err = f.fs.MkdirAll(appDir, 0755)
	f.started = err == nil
	return false, err
}

func (f *aferoCopier) Copy(slug, version, name string, src io.Reader) (err error) {
	if !f.started {
		return errors.New("copier should call Start() before Copy()")
	}
	dir := path.Dir(name)
	if err = f.fs.MkdirAll(dir, 0755); err != nil {
		return err
	}
	dst, err := f.fs.Create(name)
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

func wrapSwiftErr(err error) error {
	if err == nil {
		return nil
	}
	if err == swift.ObjectNotFound {
		return os.ErrNotExist
	}
	return nil
}
