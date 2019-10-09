package dynamic

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cozy/cozy-stack/pkg/assets/model"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/ncw/swift"
	"github.com/spf13/afero"
)

var assetFS AssetFS

// DynamicAssetsContainerName is the Swift container name for dynamic assets
const DynamicAssetsContainerName = "__dyn-assets__"

// DynamicAssetsContainerName is the folder name for dynamic assets
const DynamicAssetsFolderName = "dyn-assets"

type AssetFS interface {
	Add(string, string, *model.Asset) error
	Get(string, string) ([]byte, error)
	Remove(string, string) error
	List() (map[string][]*model.Asset, error)
	CheckStatus() error
}

type SwiftFS struct {
	swiftConn *swift.Connection
}

type AferoFS struct {
	fs     afero.Fs
	folder *url.URL
}

func (a *AferoFS) GetAssetFolderName(context, name string) string {
	return filepath.Join(a.folder.Path, context, name)
}

// Initializes de dynamic asset fs
func InitDynamicAssetFS() error {
	var err error
	scheme := config.FsURL().Scheme

	switch scheme {
	case config.SchemeFile, config.SchemeMem:
		assetFS, err = newOsFS()
		if err != nil {
			return err
		}
	case config.SchemeSwift, config.SchemeSwiftSecure:
		assetFS, err = newSwiftFS()
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("Invalid scheme %s for dynamic assets FS", scheme)
	}

	return nil
}

func newOsFS() (*AferoFS, error) {
	tmp := config.FsURL().String()
	folder, err := url.Parse(tmp)
	folder.Path = filepath.Join(folder.Path, DynamicAssetsFolderName)

	if err != nil {
		return nil, err
	}

	aferoFS := &AferoFS{fs: afero.NewOsFs(), folder: folder}
	if err := aferoFS.fs.MkdirAll(aferoFS.folder.Path, 0755); err != nil && !os.IsExist(err) {
		return nil, err
	}
	return aferoFS, nil
}

func newSwiftFS() (*SwiftFS, error) {
	swiftFS := &SwiftFS{swiftConn: config.GetSwiftConnection()}
	err := swiftFS.swiftConn.ContainerCreate(DynamicAssetsContainerName, nil)
	if err != nil {
		return nil, err
	}

	return swiftFS, nil
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

	_, err = f.Write(asset.GetUnzippedData())
	if err != nil {
		return err
	}

	return f.Close()
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

func (a *AferoFS) Remove(context, name string) error {
	filePath := a.GetAssetFolderName(context, name)
	return a.fs.Remove(filePath)
}

func (a *AferoFS) CheckStatus() error {
	_, err := a.fs.Stat("/")
	return err
}

func (a *AferoFS) List() (map[string][]*model.Asset, error) {
	objs := map[string][]*model.Asset{}

	// List contexts
	entries, err := ioutil.ReadDir(a.folder.Path)
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

func (s *SwiftFS) Add(context, name string, asset *model.Asset) error {
	objectName := path.Join(asset.Context, asset.Name)
	swiftConn := s.swiftConn
	f, err := swiftConn.ObjectCreate(DynamicAssetsContainerName, objectName, true, "", "", nil)
	if err != nil {
		return err
	}

	// Writing the asset content to Swift
	_, err = f.Write(asset.GetUnzippedData())
	if err != nil {
		return err
	}
	return f.Close()
}

func (s *SwiftFS) Get(context, name string) ([]byte, error) {
	objectName := path.Join(context, name)
	assetContent := new(bytes.Buffer)

	_, err := s.swiftConn.ObjectGet(DynamicAssetsContainerName, objectName, assetContent, true, nil)
	if err != nil {
		return nil, err
	}

	return assetContent.Bytes(), nil
}

func (s *SwiftFS) Remove(context, name string) error {
	objectName := path.Join(context, name)

	return s.swiftConn.ObjectDelete(DynamicAssetsContainerName, objectName)
}

func (s *SwiftFS) List() (map[string][]*model.Asset, error) {
	objs := map[string][]*model.Asset{}

	objNames, err := s.swiftConn.ObjectNamesAll(DynamicAssetsContainerName, nil)
	if err != nil {
		return nil, err
	}

	for _, obj := range objNames {
		splitted := strings.SplitN(obj, "/", 2)
		ctx := splitted[0]
		assetName := model.NormalizeAssetName(splitted[1])

		a, err := GetAsset(ctx, assetName)
		if err != nil {
			return nil, err
		}

		objs[ctx] = append(objs[ctx], a)
	}

	return objs, nil
}

func (s *SwiftFS) CheckStatus() error {
	_, _, err := s.swiftConn.Container(DynamicAssetsContainerName)
	return err
}
