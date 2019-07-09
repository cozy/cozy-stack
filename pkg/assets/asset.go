package assets

import (
	"bytes"
	"fmt"
	"os"

	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	"github.com/cozy/cozy-stack/pkg/assets/model"
	"github.com/cozy/cozy-stack/pkg/assets/statik"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/logger"
)

// Get looks for an asset. First tries to get a dynamic, then the statik
// Get returns an asset for the given context, or the default context if
// no context is given.
func Get(name string, context ...string) (*model.Asset, bool) {
	var ctx string

	if len(context) > 0 && context[0] != "" {
		ctx = context[0]
	} else {
		ctx = config.DefaultInstanceContext
	}

	// Check if a dynamic asset is existing
	dynAsset, err := dynamic.GetAsset(ctx, name)
	if err == nil {
		return dynAsset, true
	}
	if err != dynamic.ErrDynAssetNotFound {
		logger.WithNamespace("asset").Errorf("Error while retreiving dynamic asset: %s", err)
	}

	if ctx != config.DefaultInstanceContext {
		dynAsset, err = dynamic.GetAsset(config.DefaultInstanceContext, name)
		if err == nil {
			return dynAsset, true
		}
	}

	// If asset was not found, try to retrieve it from static assets
	asset := statik.GetAsset(name)
	if asset == nil {
		return nil, false
	}
	return asset, true
}

// Remove removes an asset
// Note: Only dynamic assets can be removed
func Remove(name, context string) error {
	if context == "" {
		return fmt.Errorf("Cannot remove a statik asset. Please specify a context")
	}

	return dynamic.RemoveAsset(context, name)
}

// List returns a map containing all the existing assets (statik & dynamic)
// Each map key represents the context
func List() (map[string][]*model.Asset, error) {
	assetsMap := make(map[string][]*model.Asset)

	defctx := config.DefaultInstanceContext

	// Get dynamic assets
	dynAssets, err := dynamic.ListAssets()
	if err != nil {
		return nil, err
	}
	for ctx, assets := range dynAssets {
		assetsMap[ctx] = append(assetsMap[ctx], assets...)
	}

	// Get statik assets
	statik.Foreach(func(name string, f *model.Asset) {
		for _, asset := range assetsMap[defctx] {
			if asset.Name == f.Name {
				return
			}
		}
		assetsMap[defctx] = append(assetsMap[defctx], f)
	})

	return assetsMap, nil
}

// Add adds dynamic assets
func Add(unmarshaledAssets []model.AssetOption) error {
	return dynamic.RegisterCustomExternals(unmarshaledAssets, 0)
}

// Open returns a bytes.Reader for an asset in the given context, or the
// default context if no context is given.
func Open(name string, context ...string) (*bytes.Reader, error) {
	f, ok := Get(name, context...)
	if ok {
		return f.Reader(), nil
	}
	return nil, os.ErrNotExist
}
