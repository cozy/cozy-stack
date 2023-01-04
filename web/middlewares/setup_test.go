package middlewares_test

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web"
	"github.com/labstack/echo/v4"
)

func TestSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile()
	config.GetConfig().Assets = "../../assets"
	setup := testutils.NewSetup(m, "middlewares_test")

	err := setup.SetupSwiftTest()
	if err != nil {
		panic("Could not init Swift test")
	}
	err = dynamic.InitDynamicAssetFS()
	if err != nil {
		panic("Could not init dynamic FS")
	}
	_ = web.SetupAssets(echo.New(), config.GetConfig().Assets)

}
