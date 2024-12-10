package vfsafero

import (
	"io"
	"net/http"
	"os"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/spf13/afero"
)

// NewAvatarFs creates a new avatar filesystem base on a afero.Fs.
func NewAvatarFs(fs afero.Fs) vfs.Avatarer {
	return &avatar{fs}
}

type avatar struct {
	fs afero.Fs
}

type avatarUpload struct {
	afero.File
	fs      afero.Fs
	tmpname string
}

func (u *avatarUpload) Close() error {
	if err := u.File.Close(); err != nil {
		_ = u.fs.Remove(u.tmpname)
		return err
	}
	return u.fs.Rename(u.tmpname, "avatar")
}

func (a *avatar) CreateAvatar(contentType string) (io.WriteCloser, error) {
	f, err := afero.TempFile(a.fs, "/", "avatar")
	if err != nil {
		return nil, err
	}
	tmpname := f.Name()
	u := &avatarUpload{
		File:    f,
		fs:      a.fs,
		tmpname: tmpname,
	}
	return u, nil
}

func (a *avatar) AvatarExists() (bool, error) {
	infos, err := a.fs.Stat("avatar")
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return infos.Size() > 0, nil
}

func (a *avatar) ServeAvatarContent(w http.ResponseWriter, req *http.Request) error {
	s, err := a.fs.Stat("avatar")
	if err != nil {
		return err
	}
	if s.Size() == 0 {
		return os.ErrInvalid
	}
	f, err := a.fs.Open("avatar")
	if err != nil {
		return err
	}
	defer f.Close()
	http.ServeContent(w, req, "avatar", s.ModTime(), f)
	return nil
}
