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

	"github.com/cozy/afero"
	"github.com/cozy/cozy-stack/pkg/assets/model"
	"github.com/cozy/cozy-stack/pkg/config/config"
)

// DynamicAssetsFolderName is the folder name for dynamic assets
const DynamicAssetsFolderName = "dyn-assets"

// OsFS is the OS implementation of [AssetsFS].
//
// It saves the assets directly on the host OS filesyteme.
//
// Technically this is a wrapper around https://github.com/spf13/afero
// which handle all the heavy works around the OS compatibility.
type OsFS struct {
	fs     afero.Fs
	folder *url.URL
}

// NewOsFS instantiate a new OsFS.
func NewOsFS() (*OsFS, error) {
	tmp := config.FsURL().String()
	folder, err := url.Parse(tmp)
	folder.Path = filepath.Join(folder.Path, DynamicAssetsFolderName)

	if err != nil {
		return nil, err
	}

	aferoFS := &OsFS{fs: afero.NewOsFs(), folder: folder}
	if err := aferoFS.fs.MkdirAll(aferoFS.folder.Path, 0755); err != nil && !os.IsExist(err) {
		return nil, err
	}
	return aferoFS, nil
}

func (a *OsFS) GetAssetFolderName(context, name string) string {
	return filepath.Join(a.folder.Path, context, name)
}

func (a *OsFS) Remove(context, name string) error {
	filePath := a.GetAssetFolderName(context, name)
	return a.fs.Remove(filePath)
}

func (a *OsFS) CheckStatus(_ context.Context) (time.Duration, error) {
	before := time.Now()
	_, err := a.fs.Stat("/")
	return time.Since(before), err
}

func (a *OsFS) List() (map[string][]*model.Asset, error) {
	objs := map[string][]*model.Asset{}

	// List contexts
	entries, err := os.ReadDir(a.folder.Path)
	if err != nil {
		return nil, err
	}
	for _, context := range entries {
		ctxName := context.Name()
		ctxPath := filepath.Join(a.folder.Path, ctxName)

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

func (a *OsFS) Get(context, name string) ([]byte, error) {
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

func (a *OsFS) Add(context, name string, asset *model.Asset) error {
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
