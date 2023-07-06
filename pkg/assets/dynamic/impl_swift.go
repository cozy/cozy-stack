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

// DynamicAssetsContainerName is the Swift container name for dynamic assets
const DynamicAssetsContainerName = "__dyn-assets__"

// SwiftFS is the Swift implementation of [AssetsFS].
//
// It save and fetch assets into/from any OpenStack Swift compatible API.
type SwiftFS struct {
	swiftConn *swift.Connection
	ctx       context.Context
}

// NewSwiftFS instantiate a new SwiftFS.
func NewSwiftFS() (*SwiftFS, error) {
	ctx := context.Background()
	swiftFS := &SwiftFS{swiftConn: config.GetSwiftConnection(), ctx: ctx}
	err := swiftFS.swiftConn.ContainerCreate(ctx, DynamicAssetsContainerName, nil)
	if err != nil {
		return nil, fmt.Errorf("Cannot create container for dynamic assets: %s", err)
	}

	return swiftFS, nil
}

func (s *SwiftFS) Add(context, name string, asset *model.Asset) error {
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

func (s *SwiftFS) Get(context, name string) ([]byte, error) {
	objectName := path.Join(context, name)
	assetContent := new(bytes.Buffer)

	_, err := s.swiftConn.ObjectGet(s.ctx, DynamicAssetsContainerName, objectName, assetContent, true, nil)
	if err != nil {
		return nil, err
	}

	return assetContent.Bytes(), nil
}

func (s *SwiftFS) Remove(context, name string) error {
	objectName := path.Join(context, name)

	return s.swiftConn.ObjectDelete(s.ctx, DynamicAssetsContainerName, objectName)
}

func (s *SwiftFS) List() (map[string][]*model.Asset, error) {
	objs := map[string][]*model.Asset{}

	opts := &swift.ObjectsOpts{Limit: 10_000}
	objNames, err := s.swiftConn.ObjectNamesAll(s.ctx, DynamicAssetsContainerName, opts)
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

func (s *SwiftFS) CheckStatus(ctx context.Context) (time.Duration, error) {
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
