package dynamic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"testing"

	"github.com/cozy/cozy-stack/pkg/assets/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAferoFS(t *testing.T) {
	addAsset := func(t *testing.T, fs *AferoFS, contextName, assetName string, content []byte) error {
		h := sha256.New()
		sum := h.Sum(content)

		opt := model.AssetOption{
			Name:     "/" + assetName,
			Context:  contextName,
			URL:      "file:/" + assetName,
			Shasum:   hex.EncodeToString(sum),
			IsCustom: true,
		}
		asset := model.NewAsset(opt, content, nil)

		return fs.Add(contextName, assetName, asset)
	}

	t.Run("GetAssetFolderName", func(t *testing.T) {
		fs := NewInMemoryFS()
		folderName := fs.GetAssetFolderName("my-context", "asset")
		assert.Equal(t, "my-context/asset", folderName)
	})

	t.Run("CheckStatus", func(t *testing.T) {
		fs := NewInMemoryFS()
		_, err := fs.CheckStatus(context.Background())
		require.NoError(t, err)
	})

	t.Run("Add and Get", func(t *testing.T) {
		fs := NewInMemoryFS()
		content := []byte("content of foo.js")
		require.NoError(t, addAsset(t, fs, "my-context", "foo.js", content))
		actual, err := fs.Get("my-context", "/foo.js")
		require.NoError(t, err)
		require.Equal(t, content, actual)
	})

	t.Run("Remove", func(t *testing.T) {
		fs := NewInMemoryFS()
		content := []byte("content of foo.js")
		require.NoError(t, addAsset(t, fs, "my-context", "foo.js", content))
		require.NoError(t, fs.Remove("my-context", "/foo.js"))
		_, err := fs.Get("my-context", "/foo.js")
		assert.Error(t, err)
	})

	t.Run("List", func(t *testing.T) {
		fs := NewInMemoryFS()
		foo := []byte("content of foo.js")
		require.NoError(t, addAsset(t, fs, "context-one", "foo.js", foo))
		bar := []byte("content of bar.js")
		require.NoError(t, addAsset(t, fs, "context-two", "bar.js", bar))
		baz := []byte("content of baz.js")
		require.NoError(t, addAsset(t, fs, "context-two", "baz.js", baz))
		list, err := fs.List()
		require.NoError(t, err)
		require.Len(t, list, 2)
		var names []string
		for contextName, assets := range list {
			for _, asset := range assets {
				assert.Equal(t, contextName, asset.AssetOption.Context)
				names = append(names, contextName+asset.AssetOption.Name)
			}
		}
		sort.Strings(names)
		expected := []string{
			"context-one/foo.js",
			"context-two/bar.js",
			"context-two/baz.js",
		}
		assert.Equal(t, expected, names)
	})
}
