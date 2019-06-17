package dynamic

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cozy/cozy-stack/pkg/cache"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/statik/fs"
	"github.com/cozy/redis"
	"github.com/stretchr/testify/assert"
)

const redisURL = "redis://localhost:6379/0"

func TestRemoveCustomAsset(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	cache := cache.New(client)

	// Cleaning if existing
	asset := fs.AssetOption{
		Name:    "/foo.js",
		Context: "bar",
	}
	_ = RemoveAsset(asset.Context, asset.Name)
	_ = UpdateAssetsList()

	assetsList, err := GetAssetsList()
	assert.NoError(t, err)

	// Adding the asset
	tmpfile, err := os.OpenFile(filepath.Join(os.TempDir(), "foo.js"), os.O_CREATE, 0600)
	assert.NoError(t, err)
	asset.URL = fmt.Sprintf("file://%s", tmpfile.Name())

	assets := []fs.AssetOption{asset}

	err = fs.RegisterCustomExternals(cache, assets, 1)
	assert.NoError(t, err)
	err = UpdateAssetsList()
	assert.NoError(t, err)
	newAssetsList, err := GetAssetsList()
	assert.NoError(t, err)
	assert.Equal(t, len(newAssetsList), len(assetsList)+1)

	// Removing
	err = RemoveAsset(asset.Context, asset.Name)
	assert.NoError(t, err)
	finalAssetsList, err := GetAssetsList()
	assert.NoError(t, err)
	assert.Equal(t, len(finalAssetsList), len(assetsList))
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	os.Exit(m.Run())
}
