package middlewares_test

import (
	"os"
	"testing"

	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web"
	"github.com/labstack/echo/v4"
)

func TestRemoveTheTemporaryWorkaround(t *testing.T) {
	testutils.TODO(t, "2021-10-05", "Remove the temporary work-around in web/middlewares/permissions.go")
}

func TestMain(m *testing.M) {
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

	os.Exit(setup.Run())
}
