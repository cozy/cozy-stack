package assets

import (
	"bytes"
	"fmt"
	"os"

	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	"github.com/cozy/cozy-stack/pkg/assets/model"
	"github.com/cozy/cozy-stack/pkg/assets/statik"
	"github.com/cozy/cozy-stack/pkg/config/config"
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

	// Dynamic assets are stored in Swift
	if config.FsURL().Scheme == config.SchemeSwift ||
		config.FsURL().Scheme == config.SchemeSwiftSecure {
		dynAsset, err := dynamic.GetAsset(ctx, name)
		if err == nil {
			return dynAsset, true
		}
	}

	// If asset was not found, try to retrieve it from static assets
	asset := statik.GetAsset(ctx, name)
	if asset == nil {
		return nil, false
	}
	return asset, true
}

// Remove removes an asset
// Note: Only dynamic assets can be removed
func Remove(name, context string) error {
	// No context
	if context == "" || context == config.DefaultInstanceContext {
		return fmt.Errorf("Cannot remove a statik asset. Please specify a context")
	}

	return dynamic.RemoveAsset(context, name)
}

// List returns a map containing all the existing assets (statik & dynamic)
// Each map key represents the context
func List() (map[string][]*model.Asset, error) {
	assetsMap := make(map[string][]*model.Asset)

	// Get statik assets
	statik.Foreach(func(name, context string, f *model.Asset) {
		assetsMap[context] = append(assetsMap[context], f)
	})

	// Get dynamic assets
	dynAssets, err := dynamic.ListAssets()
	if err != nil {
		return nil, err
	}
	for ctx, assets := range dynAssets {
		assetsMap[ctx] = append(assetsMap[ctx], assets...)
	}
	return assetsMap, nil
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
