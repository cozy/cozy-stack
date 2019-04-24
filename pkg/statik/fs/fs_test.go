package fs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/cozy/cozy-stack/pkg/cache"
	"github.com/go-redis/redis"
	"github.com/stretchr/testify/assert"
)

const redisURL = "redis://localhost:6379/0"

func TestAddCustomAsset(t *testing.T) {
	var err error
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	cache := cache.New(client)

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

	err = registerCustomExternal(cache, a)
	assert.NoError(t, err)
	asset, ok := globalAssets.Load(marshalContextKey("bar", "/foo.js"))

	assert.True(t, ok)
	assert.True(t, asset.(*Asset).IsCustom)
	assert.Equal(t, asset.(*Asset).Shasum, a.Shasum)

	globalAssets.Delete(marshalContextKey("bar", "/foo.js"))
}

func TestAddCustomAssetEmptyContext(t *testing.T) {
	var err error
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	cache := cache.New(client)

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

	assert.NoError(t, registerCustomExternal(cache, a))
	asset, ok := globalAssets.Load(marshalContextKey("bar", "/foo.js"))
	assert.False(t, ok)
	assert.Nil(t, asset)
}
