package middlewares_test

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile()
	config.GetConfig().Assets = "../../assets"
	setup := testutils.NewSetup(t, t.Name())

	require.NoError(t, setup.SetupSwiftTest(), "Could not init Swift test")
	require.NoError(t, dynamic.InitDynamicAssetFS(config.FsURL().String()), "Could not init dynamic FS")

	_ = web.SetupAssets(echo.New(), config.GetConfig().Assets)
}
