package dynamic

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/assets/model"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/spf13/afero"
)

// DynamicAssetsFolderName is the folder name for dynamic assets
const DynamicAssetsFolderName = "dyn-asets"

// AferoFS is a wrapper around the [spf13/afero] filesystem.
//
// It can be setup with two differents drivers:
//   - [NewInMemory] use the in-memory driver. It should be
//     used only for the tests as nothing is persisted.
//   - [NewOsFS] use the OsFs driver. It will save the assets
//     on the host filesystem.
//
// [spf13/afero]: https://github.com/spf13/afero
type AferoFS struct {
	fs     afero.Fs
	folder string
}

// NewInMemoryFS instantiate a new [AferoFS] with the in-memeory driver.
//
// This implementation loose every data after being clean up so it should
// be only used for the tests.
func NewInMemoryFS() *AferoFS {
	return &AferoFS{fs: afero.NewMemMapFs()}
}

// NewOsFS instantiate a new [AferoFS] with the OsFS driver.
func NewOsFS() (*AferoFS, error) {
	tmp := config.FsURL().String()
	folder, err := url.Parse(tmp)
	folder.Path = filepath.Join(folder.Path, DynamicAssetsFolderName)

	if err != nil {
		return nil, err
	}

	aferoFS := &AferoFS{fs: afero.NewOsFs(), folder: folder.Path}
	if err := aferoFS.fs.MkdirAll(aferoFS.folder, 0755); err != nil && !os.IsExist(err) {
		return nil, err
	}
	return aferoFS, nil
}

func (a *AferoFS) GetAssetFolderName(context, name string) string {
	return filepath.Join(a.folder, context, name)
}

func (a *AferoFS) Remove(context, name string) error {
	filePath := a.GetAssetFolderName(context, name)
	return a.fs.Remove(filePath)
}

func (a *AferoFS) CheckStatus(_ context.Context) (time.Duration, error) {
	before := time.Now()
	_, err := a.fs.Stat("/")
	return time.Since(before), err
}

func (a *AferoFS) List() (map[string][]*model.Asset, error) {
	objs := map[string][]*model.Asset{}

	// List contexts
	entries, err := os.ReadDir(a.folder)
	if err != nil {
		return nil, err
	}
	for _, context := range entries {
		ctxName := context.Name()
		ctxPath := filepath.Join(a.folder, ctxName)

		err := filepath.Walk(ctxPath, func(path string, info os.FileInfo, err error) error {
			if !info.IsDir() {
				assetName := strings.Replace(path, ctxPath, "", 1)
				asset, err := GetAsset(ctxName, assetName)
				if err != nil {
					return err
				}
				objs[ctxName] = append(objs[ctxName], asset)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return objs, nil
}

func (a *AferoFS) Get(context, name string) ([]byte, error) {
	filePath := a.GetAssetFolderName(context, name)

	f, err := a.fs.Open(filePath)
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)

	_, err = io.Copy(buf, f)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), f.Close()
}

func (a *AferoFS) Add(context, name string, asset *model.Asset) error {
	filePath := a.GetAssetFolderName(context, name)

	// Creates the asset folder
	err := a.fs.MkdirAll(filepath.Dir(filePath), 0755)
	if err != nil {
		return err
	}

	// Writing the file
	f, err := a.fs.OpenFile(filePath, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0600)
	if err != nil {
		return err
	}

	_, err = f.Write(asset.GetData())
	if err != nil {
		return err
	}

	return f.Close()
}
