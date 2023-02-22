package dynamic

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/assets/model"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/ncw/swift/v2"
)

var assetFS assetsFS

// DynamicAssetsContainerName is the Swift container name for dynamic assets
const DynamicAssetsContainerName = "__dyn-assets__"

// DynamicAssetsFolderName is the folder name for dynamic assets
const DynamicAssetsFolderName = "dyn-assets"

type assetsFS interface {
	Add(string, string, *model.Asset) error
	Get(string, string) ([]byte, error)
	Remove(string, string) error
	List() (map[string][]*model.Asset, error)
	CheckStatus(ctx context.Context) (time.Duration, error)
}

type swiftFS struct {
	swiftConn *swift.Connection
	ctx       context.Context
}

// InitDynamicAssetFS initializes the dynamic asset FS.
func InitDynamicAssetFS() error {
	var err error
	scheme := config.FsURL().Scheme

	switch scheme {
	case config.SchemeFile, config.SchemeMem:
		assetFS, err = NewOsFS()
		if err != nil {
			return err
		}
	case config.SchemeSwift, config.SchemeSwiftSecure:
		assetFS, err = newswiftFS()
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("Invalid scheme %s for dynamic assets FS", scheme)
	}

	return nil
}

func newswiftFS() (*swiftFS, error) {
	ctx := context.Background()
	swiftFS := &swiftFS{swiftConn: config.GetSwiftConnection(), ctx: ctx}
	err := swiftFS.swiftConn.ContainerCreate(ctx, DynamicAssetsContainerName, nil)
	if err != nil {
		return nil, fmt.Errorf("Cannot create container for dynamic assets: %s", err)
	}

	return swiftFS, nil
}

func (s *swiftFS) Add(context, name string, asset *model.Asset) error {
	objectName := path.Join(asset.Context, asset.Name)
	swiftConn := s.swiftConn
	f, err := swiftConn.ObjectCreate(s.ctx, DynamicAssetsContainerName, objectName, true, "", "", nil)
	if err != nil {
		return err
	}

	// Writing the asset content to Swift
	_, err = f.Write(asset.GetData())
	if err != nil {
		return err
	}
	return f.Close()
}

func (s *swiftFS) Get(context, name string) ([]byte, error) {
	objectName := path.Join(context, name)
	assetContent := new(bytes.Buffer)

	_, err := s.swiftConn.ObjectGet(s.ctx, DynamicAssetsContainerName, objectName, assetContent, true, nil)
	if err != nil {
		return nil, err
	}

	return assetContent.Bytes(), nil
}

func (s *swiftFS) Remove(context, name string) error {
	objectName := path.Join(context, name)

	return s.swiftConn.ObjectDelete(s.ctx, DynamicAssetsContainerName, objectName)
}

func (s *swiftFS) List() (map[string][]*model.Asset, error) {
	objs := map[string][]*model.Asset{}

	objNames, err := s.swiftConn.ObjectNamesAll(s.ctx, DynamicAssetsContainerName, nil)
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

func (s *swiftFS) CheckStatus(ctx context.Context) (time.Duration, error) {
	before := time.Now()
	var err error
	if config.GetConfig().Fs.CanQueryInfo {
		_, err = s.swiftConn.QueryInfo(ctx)
	} else {
		_, _, err = s.swiftConn.Container(ctx, DynamicAssetsContainerName)
	}
	if err != nil {
		return 0, err
	}
	return time.Since(before), nil
}
