// Package files is for storing files on the cozy, including binary ones like
// photos and movies. The range of possible operations with this endpoint goes
// from simple ones, like uploading a file, to more complex ones, like renaming
// a folder. It also ensure that an instance is not exceeding its quota, and
// keeps a trash to recover files recently deleted.
package files

import (
	"bytes"
	"io"

	"github.com/spf13/afero"
)

// Upload is the method for uploading a file
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
	return copyOnFsAndCheck(m, fs, path, body)
}

func copyOnFsAndCheck(m *DocMetadata, fs afero.Fs, path string, r io.Reader) (err error) {
	file, err := fs.Create(path)
	if err != nil {
		return
	}
	defer file.Close()

	md5H := m.md5H
	done := m.doneCh
	errs := m.errsCh
	defer close(done)
	defer close(errs)

	docR, docW := io.Pipe()
	md5R, md5W := io.Pipe()

	go doCopy(file, docR, done, errs)
	go doCopy(md5H, md5R, done, errs)

	go func() {
		defer docW.Close()
		defer md5W.Close()

		mw := io.MultiWriter(docW, md5W)

		_, err = io.Copy(mw, r)
		if err != nil {
			errs <- err
		}
	}()

	for c := 0; c < 2; c++ {
		select {
		case <-done:
		case err = <-errs:
			return
		}
	}

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

func doCopy(dst io.Writer, src io.Reader, done chan bool, errs chan error) {
	_, err := io.Copy(dst, src)
	if err != nil {
		errs <- err
	} else {
		done <- true
	}
}
