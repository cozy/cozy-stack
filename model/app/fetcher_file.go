package app

import (
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"

	"github.com/cozy/cozy-stack/pkg/appfs"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/sirupsen/logrus"
)

type fileFetcher struct {
	manFilename string
	log         *logrus.Entry
}

// The file fetcher is mostly used in development mode. The version of the
// application installed with this mode is appended with a random number so
// that multiple version can be installed from the same directory without
// having to increase the version number from the manifest.
func newFileFetcher(manFilename string, log *logrus.Entry) *fileFetcher {
	return &fileFetcher{
		manFilename: manFilename,
		log:         log,
	}
}

func (f *fileFetcher) FetchManifest(src *url.URL) (io.ReadCloser, error) {
	r, err := os.Open(filepath.Join(src.Path, f.manFilename))
	if os.IsNotExist(err) {
		return nil, ErrManifestNotReachable
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (f *fileFetcher) Fetch(src *url.URL, fs appfs.Copier, man Manifest) (err error) {
	version := man.Version() + "-" + utils.RandomString(10)
	man.SetVersion(version)
	exists, err := fs.Start(man.Slug(), man.Version(), "")
	if err != nil || exists {
		return err
	}
	defer func() {
		if err != nil {
			_ = fs.Abort()
		} else {
			err = fs.Commit()
		}
	}()
	return copyRec(src.Path, "/", fs)
}

func copyRec(root, path string, fs appfs.Copier) error {
	files, err := ioutil.ReadDir(filepath.Join(root, path))
	if err != nil {
		return err
	}
	for _, file := range files {
		relpath := filepath.Join(path, file.Name())
		if file.IsDir() {
			if file.Name() == ".git" {
				continue
			}
			if err = copyRec(root, relpath, fs); err != nil {
				return err
			}
			continue
		}
		fullpath := filepath.Join(root, path, file.Name())
		f, err := os.Open(fullpath)
		if err != nil {
			return err
		}
		defer f.Close()
		info := appfs.NewFileInfo(relpath, file.Size(), file.Mode())
		err = fs.Copy(info, f)
		if err != nil {
			return err
		}
	}
	return nil
}
