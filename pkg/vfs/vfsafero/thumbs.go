package vfsafero

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/cozy/cozy-stack/pkg/vfs"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/spf13/afero"
)

// NewThumbsFs creates a new thumb filesystem base on a afero.Fs.
func NewThumbsFs(fs afero.Fs) vfs.Thumbser {
	return &thumbs{fs}
}

type thumbs struct {
	fs afero.Fs
}

func (t *thumbs) CreateThumb(img *vfs.FileDoc, format string) (io.WriteCloser, error) {
	name := t.makeName(img, format)
	if base := path.Dir(name); base != "." {
		if err := t.fs.MkdirAll(path.Dir(name), 0755); err != nil {
			return nil, err
		}
	}
	return t.fs.OpenFile(name, os.O_WRONLY|os.O_CREATE, 0640)
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
