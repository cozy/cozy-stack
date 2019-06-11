package dynamic

import (
	"path"
	"strings"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/statik/fs"
)

const assetsListID = "assets"

// AssetsList contains the list of assets options that are loaded at the
// startup of the stack.
//
// These assets are either loaded from a persistent cache or loaded directly
// using their source URL. See statik/fs package for more informations.
type AssetsList struct {
	AssetsID   string           `json:"_id,omitempty"`
	AssetsRev  string           `json:"_rev,omitempty"`
	AssetsList []fs.AssetOption `json:"assets_list"`
}

// ID implements the couchdb.Doc interface
func (a *AssetsList) ID() string { return assetsListID }

// Rev implements the couchdb.Doc interface
func (a *AssetsList) Rev() string { return a.AssetsRev }

// DocType implements the couchdb.Doc interface
func (a *AssetsList) DocType() string { return consts.Configs }

// Clone implements the couchdb.Doc interface
func (a *AssetsList) Clone() couchdb.Doc {
	clone := *a
	clone.AssetsList = make([]fs.AssetOption, len(a.AssetsList))
	copy(clone.AssetsList, a.AssetsList)
	return &clone
}

// SetID implements the couchdb.Doc interface
func (a *AssetsList) SetID(id string) { a.AssetsID = id }

// SetRev implements the couchdb.Doc interface
func (a *AssetsList) SetRev(rev string) { a.AssetsRev = rev }

var _ couchdb.Doc = &AssetsList{}

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

// GetAssetsList fetches the Swift to get the assets provided
func GetAssetsList() ([]fs.AssetOption, error) {
	var doc AssetsList
	if err := couchdb.GetDoc(couchdb.GlobalDB, consts.Configs, assetsListID, &doc); err != nil {
		if !couchdb.IsNoDatabaseError(err) && !couchdb.IsNotFoundError(err) {
			return nil, err
		}
	}
	return doc.AssetsList, nil
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
