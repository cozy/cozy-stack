package fs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/swift"
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

	a := AssetOption{
		Name:    "/foo.js",
		Context: "bar",
		URL:     fmt.Sprintf("file:%s", tmpfile.Name()),
		Shasum:  hex.EncodeToString(sum),
	}

	err = registerCustomExternal(a)
	assert.NoError(t, err)
	asset, ok := Get("/foo.js", "bar")

	assert.True(t, ok)
	assert.True(t, asset.IsCustom)
	assert.Equal(t, asset.Shasum, a.Shasum)

	var buf = new(bytes.Buffer)
	_, err = config.GetSwiftConnection().ObjectGet(DynamicAssetsContainerName, "bar/foo.js", buf, true, nil)
	assert.NoError(t, err)

}

func TestAddCustomAssetEmptyContext(t *testing.T) {
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

	a := AssetOption{
		Name:   "/foo.js",
		URL:    fmt.Sprintf("file:%s", tmpfile.Name()),
		Shasum: hex.EncodeToString(sum),
	}

	assert.NoError(t, registerCustomExternal(a))
	var buf = new(bytes.Buffer)
	_, err = config.GetSwiftConnection().ObjectGet(DynamicAssetsContainerName, "bar/foo.js", buf, true, nil)
	assert.Error(t, err)
	assert.Equal(t, swift.ObjectNotFound, err)
}

func TestMain(m *testing.M) {
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
}
