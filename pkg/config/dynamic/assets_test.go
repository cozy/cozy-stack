package dynamic

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/statik/fs"
	"github.com/cozy/swift/swifttest"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestRemoveCustomAsset(t *testing.T) {
	// Cleaning if existing
	asset := fs.AssetOption{
		Name:    "/foo.js",
		Context: "bar",
	}

	assetsList, err := ListDynamicAssets()
	assert.NoError(t, err)

	// Adding the asset
	tmpfile, err := os.OpenFile(filepath.Join(os.TempDir(), "foo.js"), os.O_CREATE, 0600)
	assert.NoError(t, err)
	asset.URL = fmt.Sprintf("file://%s", tmpfile.Name())

	assets := []fs.AssetOption{asset}

	err = fs.RegisterCustomExternals(assets, 1)
	assert.NoError(t, err)

	assert.NoError(t, err)
	newAssetsList, err := ListDynamicAssets()
	assert.NoError(t, err)
	assert.Equal(t, len(newAssetsList), len(assetsList)+1)

	// Removing
	err = RemoveDynamicAsset(asset.Context, asset.Name)
	assert.NoError(t, err)
	finalAssetsList, err := ListDynamicAssets()
	assert.NoError(t, err)
	assert.Equal(t, len(finalAssetsList), len(assetsList))
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	swiftSrv, err := swifttest.NewSwiftServer("localhost")
	if err != nil {
		fmt.Printf("failed to create swift server %s", err)
	}

	viper.Set("swift.username", "swifttest")
	viper.Set("swift.api_key", "swifttest")
	viper.Set("swift.auth_url", swiftSrv.AuthURL)

	err = config.InitSwiftConnection(config.Fs{
		URL: &url.URL{
			Scheme:   "swift",
			Host:     "localhost",
			RawQuery: "UserName=swifttest&Password=swifttest&AuthURL=" + url.QueryEscape(swiftSrv.AuthURL),
		},
	})

	err = config.GetSwiftConnection().ContainerCreate(fs.DynamicAssetsContainerName, nil)
	os.Exit(m.Run())
}
