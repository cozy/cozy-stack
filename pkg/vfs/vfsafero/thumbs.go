package vfsafero

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/spf13/afero"
)

var unixZeroEpoch = time.Time{}

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

func (t *thumbs) RemoveThumb(img *vfs.FileDoc, format string) error {
	return t.fs.Remove(t.makeName(img, format))
}

func (t *thumbs) ServeThumbContent(w http.ResponseWriter, req *http.Request,
	img *vfs.FileDoc, format string) error {
	name := t.makeName(img, format)
	data, err := afero.ReadFile(t.fs, name)
	if err != nil {
		return err
	}
	sum := md5sum(data)
	w.Header().Set("Etag", fmt.Sprintf(`"%x"`, sum))
	http.ServeContent(w, req, name, unixZeroEpoch, bytes.NewReader(data))
	return nil
}

func md5sum(b []byte) []byte {
	h := md5.New()
	h.Write(b)
	return h.Sum(nil)
}

func (t *thumbs) makeName(img *vfs.FileDoc, format string) string {
	dir := img.ID()[:4]
	name := fmt.Sprintf("%s-%s.jpg", img.ID(), format)
	return path.Join("/", dir, name)
}
