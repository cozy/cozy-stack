package dynamic

import (
	"path"
	"strings"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/statik/fs"
)

// List dynamic assets
func ListDynamicAssets() (map[string][]*fs.Asset, error) {
	swiftConn := config.GetSwiftConnection()

	objs := map[string][]*fs.Asset{}

	objNames, err := swiftConn.ObjectNamesAll(fs.DynamicAssetsContainerName, nil)
	if err != nil {
		return nil, err
	}

	for _, obj := range objNames {
		splitted := strings.SplitN(obj, "/", 2)
		ctx := splitted[0]
		assetName := fs.NormalizeAssetName(splitted[1])

		a, err := fs.GetDynamicAsset(ctx, assetName)
		if err != nil {
			return nil, err
		}

		objs[ctx] = append(objs[ctx], a)

	}

	return objs, nil
}

// RemoveAsset removes a dynamic asset from Swift
func RemoveAsset(context, name string) error {
	swiftConn := config.GetSwiftConnection()
	objectName := path.Join(context, name)

	return swiftConn.ObjectDelete(fs.DynamicAssetsContainerName, objectName)
}

// Initializes the Swift container for dynamic assets
func InitDynamicAssetContainer() error {
	swiftConn := config.GetSwiftConnection()
	return swiftConn.ContainerCreate(fs.DynamicAssetsContainerName, nil)
}
