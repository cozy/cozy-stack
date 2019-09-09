package assets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	"github.com/cozy/cozy-stack/pkg/assets/model"
	"github.com/cozy/cozy-stack/pkg/assets/statik"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/logger"
)

// Get looks for an asset. It tries in this order:
// 1. A dynamic asset for the given context
// 2. A dynamic asset for the default context
// 3. A static asset.
func Get(name, context string) (*model.Asset, bool) {
	if context == "" {
		context = config.DefaultInstanceContext
	}

	// Check if a dynamic asset is existing
	dynAsset, err := dynamic.GetAsset(context, name)
	if err == nil {
		return dynAsset, true
	}
	if err != dynamic.ErrDynAssetNotFound {
		logger.WithNamespace("asset").Errorf("Error while retreiving dynamic asset: %s", err)
	}

	if context != config.DefaultInstanceContext {
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

// Head does the same job as Get, but the returned model.Asset can have no body
// data. It allows to use a cache for it.
func Head(name, context string) (*model.Asset, bool) {
	if context == "" {
		context = config.DefaultInstanceContext
	}
	key := fmt.Sprintf("dyn-assets:%s/%s", context, name)
	cache := config.GetConfig().CacheStorage
	if r, ok := cache.Get(key); ok {
		asset := &model.Asset{}
		if err := json.NewDecoder(r).Decode(asset); err == nil {
			return asset, true
		}
	}
	asset, ok := Get(name, context)
	if !ok {
		return nil, false
	}
	if data, err := json.Marshal(asset); err == nil {
		cache.Set(key, data, 24*time.Hour)
	}
	return asset, true
}

// Add adds dynamic assets
func Add(options []model.AssetOption) error {
	err := dynamic.RegisterCustomExternals(options, 0)
	if err == nil {
		cache := config.GetConfig().CacheStorage
		for _, opt := range options {
			key := fmt.Sprintf("dyn-assets:%s/%s", opt.Context, opt.Name)
			cache.Clear(key)
		}
	}
	return err
}

// Remove removes an asset
// Note: Only dynamic assets can be removed
func Remove(name, context string) error {
	err := dynamic.RemoveAsset(context, name)
	if err == nil {
		key := fmt.Sprintf("dyn-assets:%s/%s", context, name)
		cache := config.GetConfig().CacheStorage
		cache.Clear(key)
	}
	return err
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

// Open returns a bytes.Reader for an asset in the given context, or the
// default context if no context is given.
func Open(name string, context string) (*bytes.Reader, error) {
	f, ok := Get(name, context)
	if ok {
		return f.Reader(), nil
	}
	return nil, os.ErrNotExist
}
