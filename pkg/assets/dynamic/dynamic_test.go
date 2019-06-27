package dynamic

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/cozy/cozy-stack/pkg/assets/model"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/swift/swifttest"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestAddCustomAsset(t *testing.T) {
	var err error

	tmpfile, err := os.OpenFile(filepath.Join(os.TempDir(), "foo.js"), os.O_CREATE, 0600)
	if err != nil {
		t.Error(err)
	}
	defer tmpfile.Close()

	h := sha256.New()
	if _, err := io.Copy(h, tmpfile); err != nil {
		log.Fatal(err)
	}
	sum := h.Sum(nil)

	a := model.AssetOption{
		Name:    "/foo.js",
		Context: "bar",
		URL:     fmt.Sprintf("file:%s", tmpfile.Name()),
		Shasum:  hex.EncodeToString(sum),
	}

	err = registerCustomExternal(a)
	assert.NoError(t, err)
	asset, err := GetAsset("bar", "/foo.js")

	assert.NoError(t, err)
	assert.True(t, asset.IsCustom)
	assert.Equal(t, asset.Shasum, a.Shasum)

	content, err := assetFS.Get("bar", "/foo.js")
	assert.NoError(t, err)
	assert.Empty(t, content)
}

func TestRemoveCustomAsset(t *testing.T) {
	// Cleaning if existing
	asset := model.AssetOption{
		Name:    "/foo2.js",
		Context: "bar",
	}

	assetsList, err := ListAssets()
	assert.NoError(t, err)

	// Adding the asset
	tmpfile, err := os.OpenFile(filepath.Join(os.TempDir(), "foo2.js"), os.O_CREATE, 0600)
	assert.NoError(t, err)
	asset.URL = fmt.Sprintf("file://%s", tmpfile.Name())

	assets := []model.AssetOption{asset}

	err = RegisterCustomExternals(assets, 1)
	assert.NoError(t, err)

	assert.NoError(t, err)
	newAssetsList, err := ListAssets()
	assert.NoError(t, err)
	assert.Equal(t, len(newAssetsList["bar"]), len(assetsList["bar"])+1)

	// Removing
	err = RemoveAsset(asset.Context, asset.Name)
	assert.NoError(t, err)
	finalAssetsList, err := ListAssets()
	assert.NoError(t, err)
	assert.Equal(t, len(finalAssetsList["bar"]), len(assetsList["bar"]))
}

func TestMain(m *testing.M) {
	config.UseTestFile()

	// We cannot use setup.SetupSwiftTest() here because testutils relies on
	// stack.Start(), resulting in a circular import
	// dynamic => testutils => stack => dynamic
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
	if err != nil {
		panic("Could not init swift connection")
	}
	err = config.GetSwiftConnection().ContainerCreate(DynamicAssetsContainerName, nil)
	if err != nil {
		panic("Could not create dynamic container")
	}
	err = InitDynamicAssetFS()
	if err != nil {
		panic("Could not initialize dynamic FS")
	}
	os.Exit(m.Run())
}
