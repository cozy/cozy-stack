package vfsafero

import (
	"fmt"
	"net/http"
	"os"
	"path"

	"github.com/cozy/afero"
	"github.com/cozy/cozy-stack/model/vfs"
	multierror "github.com/hashicorp/go-multierror"
)

// NewThumbsFs creates a new thumb filesystem base on a afero.Fs.
func NewThumbsFs(fs afero.Fs) vfs.Thumbser {
	return &thumbs{fs}
}

type thumbs struct {
	fs afero.Fs
}

type thumb struct {
	afero.File
	fs      afero.Fs
	tmpname string
	newname string
}

func (t *thumb) Abort() error {
	return t.fs.Remove(t.tmpname)
}

func (t *thumb) Commit() error {
	return t.fs.Rename(t.tmpname, t.newname)
}

func (t *thumbs) CreateThumb(img *vfs.FileDoc, format string) (vfs.ThumbFiler, error) {
	newname := t.makeName(img, format)
	dir := path.Dir(newname)
	if base := dir; base != "." {
		if err := t.fs.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}
	f, err := afero.TempFile(t.fs, dir, "cozy-thumb")
	if err != nil {
		return nil, err
	}
	tmpname := f.Name()
	th := &thumb{
		File:    f,
		fs:      t.fs,
		tmpname: tmpname,
		newname: newname,
	}
	return th, nil
}

func (t *thumbs) RemoveThumbs(img *vfs.FileDoc, formats []string) error {
	var errm error
	for _, format := range formats {
		if err := t.fs.Remove(t.makeName(img, format)); err != nil && !os.IsNotExist(err) {
			errm = multierror.Append(errm, err)
		}
	}
	return errm
}

func (t *thumbs) ThumbExists(img *vfs.FileDoc, format string) (bool, error) {
	name := t.makeName(img, format)
	infos, err := t.fs.Stat(name)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return infos.Size() > 0, nil
}

func (t *thumbs) ServeThumbContent(w http.ResponseWriter, req *http.Request,
	img *vfs.FileDoc, format string) error {
	name := t.makeName(img, format)
	s, err := t.fs.Stat(name)
	if err != nil {
		return err
	}
	f, err := t.fs.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	http.ServeContent(w, req, name, s.ModTime(), f)
	return nil
}

func (t *thumbs) makeName(img *vfs.FileDoc, format string) string {
	dir := img.ID()[:4]
	name := fmt.Sprintf("%s-%s.jpg", img.ID(), format)
	return path.Join("/", dir, name)
}
