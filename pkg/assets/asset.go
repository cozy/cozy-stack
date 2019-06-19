package assets

import (
	"bytes"
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

// Open returns a bytes.Reader for an asset in the given context, or the
// default context if no context is given.
func Open(name string, context ...string) (*bytes.Reader, error) {
	f, ok := Get(name, context...)
	if ok {
		return f.Reader(), nil
	}
	return nil, os.ErrNotExist
}
