package vfsafero

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
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
	newname := t.makeName(img.ID(), format)
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
		if err := t.fs.Remove(t.makeName(img.ID(), format)); err != nil && !os.IsNotExist(err) {
			errm = multierror.Append(errm, err)
		}
	}
	return errm
}

func (t *thumbs) ThumbExists(img *vfs.FileDoc, format string) (bool, error) {
	name := t.makeName(img.ID(), format)
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
	name := t.makeName(img.ID(), format)
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

func (t *thumbs) CreateNoteThumb(id, mime, format string) (vfs.ThumbFiler, error) {
	newname := t.makeName(id, format)
	dir := path.Dir(newname)
	if base := dir; base != "." {
		if err := t.fs.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}
	f, err := afero.TempFile(t.fs, dir, "note-thumb")
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

func (t *thumbs) OpenNoteThumb(id, format string) (io.ReadCloser, error) {
	name := t.makeName(id, format)
	return t.fs.Open(name)
}

func (t *thumbs) RemoveNoteThumb(id string, formats []string) error {
	var errm error
	for _, format := range formats {
		err := t.fs.Remove(t.makeName(id, format))
		if err != nil && !os.IsNotExist(err) {
			errm = multierror.Append(errm, err)
		}
	}
	return errm
}

func (t *thumbs) ServeNoteThumbContent(w http.ResponseWriter, req *http.Request, id string) error {
	name := t.makeName(id, consts.NoteImageThumbFormat)
	s, err := t.fs.Stat(name)
	if err != nil {
		name = t.makeName(id, consts.NoteImageOriginalFormat)
		s, err = t.fs.Stat(name)
		if err != nil {
			return err
		}
	}
	f, err := t.fs.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	http.ServeContent(w, req, name, s.ModTime(), f)
	return nil
}

func (t *thumbs) makeName(imgID string, format string) string {
	dir := imgID[:4]
	ext := ".jpg"
	if format == consts.NoteImageOriginalFormat {
		ext = ""
	}
	name := fmt.Sprintf("%s-%s%s", imgID, format, ext)
	return path.Join("/", dir, name)
}
