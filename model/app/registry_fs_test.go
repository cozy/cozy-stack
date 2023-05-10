package app

import (
	"os"
	"testing"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalRegistry_Add_App_ok(t *testing.T) {
	fs := afero.NewMemMapFs()
	fs.Mkdir("/someApp", 0o755)
	fs.Create("/someApp/manifest.webapp")
	fs.Create("/someApp/index.html")

	registry := NewLocalRegistry(fs)

	res := registry.List()
	assert.Len(t, res, 0)

	err := registry.Add("some-app", LocalResource{
		Type: consts.WebappType,
		Dir:  "/someApp",
	})
	require.NoError(t, err)

	res = registry.List()
	assert.Len(t, res, 1)
}

func TestLocalRegistry_Add_Konnector_ok(t *testing.T) {
	fs := afero.NewMemMapFs()
	fs.Mkdir("/someKonn", 0o755)
	fs.Create("/someKonn/manifest.konnector")

	registry := NewLocalRegistry(fs)

	res := registry.List()
	assert.Len(t, res, 0)

	err := registry.Add("some-konn", LocalResource{
		Type: consts.KonnectorType,
		Dir:  "/someKonn",
	})
	require.NoError(t, err)

	res = registry.List()
	assert.Len(t, res, 1)
}

func TestLocalRegistry_Add_with_folder_not_found(t *testing.T) {
	// We don't have any folder in fs.
	fs := afero.NewMemMapFs()

	registry := NewLocalRegistry(fs)

	res := registry.List()
	assert.Len(t, res, 0)

	err := registry.Add("some-app", LocalResource{
		Type: consts.KonnectorType,
		Dir:  "/someKonn",
	})
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestLocalRegistry_Add_with_missing_manifest(t *testing.T) {
	fs := afero.NewMemMapFs()
	fs.Mkdir("/someKonn", 0o755)

	registry := NewLocalRegistry(fs)

	res := registry.List()
	assert.Len(t, res, 0)

	err := registry.Add("someKonn", LocalResource{
		Type: consts.KonnectorType,
		Dir:  "/someKonn",
	})
	assert.ErrorIs(t, err, ErrInvalidManifestPath)
}

func TestLocalRegistry_Add_with_missing_index_html(t *testing.T) {
	fs := afero.NewMemMapFs()

	fs.Mkdir("/someApp", 0o755)
	fs.Create("/someApp/manifest.webapp")

	registry := NewLocalRegistry(fs)

	res := registry.List()
	assert.Len(t, res, 0)

	err := registry.Add("some-app", LocalResource{
		Type: consts.WebappType,
		Dir:  "/someApp",
	})
	assert.ErrorIs(t, err, ErrInvalidIndexPath)
}

func TestLocalRegistry_GetKonnManifest_ok(t *testing.T) {
	fs := afero.NewReadOnlyFs(afero.NewOsFs())
	fs = afero.NewBasePathFs(fs, "./testdata")

	registry := NewLocalRegistry(fs)

	err := registry.Add("konn-1", LocalResource{
		Type: consts.KonnectorType,
		Dir:  "/konnFolder",
	})
	require.NoError(t, err)

	manifest, err := registry.GetKonnManifest("konn-1")
	require.NoError(t, err)

	assert.Equal(t, "konn-1", manifest.Slug())
	assert.Equal(t, "1.9.0", manifest.Version())
	assert.Equal(t, "node", manifest.Language())
	assert.Equal(t, "icon.svg", manifest.Icon())
}

func TestLocalRegistry_GetWebAppManifest_ok(t *testing.T) {
	fs := afero.NewReadOnlyFs(afero.NewOsFs())
	fs = afero.NewBasePathFs(fs, "./testdata")

	registry := NewLocalRegistry(fs)

	err := registry.Add("app-1", LocalResource{
		Type: consts.WebappType,
		Dir:  "/appFolder",
	})
	require.NoError(t, err)

	manifest, err := registry.GetWebAppManifest("app-1")
	require.NoError(t, err)

	assert.Equal(t, "app-1", manifest.Slug())
	assert.Equal(t, "1.54.0", manifest.Version())
	assert.Equal(t, consts.WebappType, manifest.AppType())
}

func TestLocalRegistry_GetKonnManifest_not_found(t *testing.T) {
	fs := afero.NewMemMapFs()

	registry := NewLocalRegistry(fs)

	manifest, err := registry.GetKonnManifest("unknown-konn")
	require.ErrorIs(t, err, ErrNotFound)
	require.Nil(t, manifest)
}

func TestLocalRegistry_GetWebAppManifest_not_found(t *testing.T) {
	fs := afero.NewMemMapFs()

	registry := NewLocalRegistry(fs)

	manifest, err := registry.GetWebAppManifest("unknown-app")
	require.ErrorIs(t, err, ErrNotFound)
	require.Nil(t, manifest)
}
