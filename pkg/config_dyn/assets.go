package config_dyn

import (
	"time"

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

func (a *AssetsList) ID() string      { return assetsListID }
func (a *AssetsList) Rev() string     { return a.AssetsRev }
func (a *AssetsList) DocType() string { return consts.Configs }

func (a *AssetsList) Clone() couchdb.Doc {
	clone := *a
	clone.AssetsList = make([]fs.AssetOption, len(a.AssetsList))
	copy(clone.AssetsList, a.AssetsList)
	return &clone
}

func (a *AssetsList) SetID(id string)   { a.AssetsID = id }
func (a *AssetsList) SetRev(rev string) { a.AssetsRev = rev }

var _ couchdb.Doc = &AssetsList{}

// GetAssetsList fetches the configuration document containing the list of
// assets required by the stack.
func GetAssetsList() ([]fs.AssetOption, error) {
	var doc AssetsList
	if err := couchdb.GetDoc(couchdb.GlobalDB, consts.Configs, assetsListID, &doc); err != nil {
		if !couchdb.IsNoDatabaseError(err) && !couchdb.IsNotFoundError(err) {
			return nil, err
		}
	}
	return doc.AssetsList, nil
}

// UpdateAssetsList updates the assets list document in CouchDB to reflect the
// current list of assets.
func UpdateAssetsList() error {
	var doc AssetsList
	fs.Foreach(func(name, context string, f *fs.Asset) {
		if f.IsCustom {
			doc.AssetsList = append(doc.AssetsList, f.AssetOption)
		}
	})
	return couchdb.Upsert(couchdb.GlobalDB, &doc)
}

// PollAssetsList executes itself in its own goroutine to poll at regular
// intervals the list of assets that should be delivered by the stack.
func PollAssetsList(cacheStorage fs.Cache, pollingInterval time.Duration) {
	if pollingInterval == 0 {
		pollingInterval = 2 * time.Minute
	}
	for {
		time.Sleep(pollingInterval)
		assetsList, err := GetAssetsList()
		if err == nil {
			fs.RegisterCustomExternals(cacheStorage, assetsList, 6 /*= retry count */)
		}
	}
}
