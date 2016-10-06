// Package files is for storing files on the cozy, including binary ones like
// photos and movies. The range of possible operations with this endpoint goes
// from simple ones, like uploading a file, to more complex ones, like renaming
// a folder. It also ensure that an instance is not exceeding its quota, and
// keeps a trash to recover files recently deleted.
package files

import (
	"bytes"
	"crypto/md5" // #nosec
	"io"

	"github.com/spf13/afero"
)

// Upload is the method for uploading a file onto the filesystem.
func Upload(m *DocMetadata, fs afero.Fs, body io.ReadCloser) (err error) {
	if m.Type != FileDocType {
		return errDocTypeInvalid
	}

	path := m.path()

	// Existence of FolderID is mandatory
	exists, err := afero.Exists(fs, path)
	if err != nil {
		return
	}
	if exists {
		return errDocAlreadyExists
	}

	defer body.Close()
	return copyOnFsAndCheckIntegrity(m, fs, path, body)
}

func copyOnFsAndCheckIntegrity(m *DocMetadata, fs afero.Fs, path string, r io.Reader) (err error) {
	file, err := fs.Create(path)
	if err != nil {
		return
	}
	defer file.Close()

	md5H := md5.New() // #nosec
	_, err = io.Copy(file, io.TeeReader(r, md5H))

	calcMD5 := md5H.Sum(nil)
	if !bytes.Equal(m.GivenMD5, calcMD5) {
		err = fs.Remove(path)
		if err == nil {
			err = errInvalidHash
		}
		return
	}

	return
}
