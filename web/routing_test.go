package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cozy/cozy-stack/model/stack"
	"github.com/cozy/cozy-stack/pkg/assets/dynamic"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouting(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	config.GetConfig().Assets = "../assets"
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	inst := setup.GetTestInstance()
	domain := inst.Domain
	require.NoError(t, dynamic.InitDynamicAssetFS(config.FsURL().String()), "Could not init dynamic FS")

	t.Run("SetupAssets", func(t *testing.T) {
		e := echo.New()
		err := SetupAssets(e, "../assets")
		require.NoError(t, err)

		ts := httptest.NewServer(e)
		t.Cleanup(ts.Close)

		t.Run("GET on an asset", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			e.GET("/assets/images/cozy.svg").
				Expect().Status(200)
		})

		t.Run("HEAD on an asset", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			e.HEAD("/assets/images/cozy.svg").
				Expect().Status(200)
		})
	})

	t.Run("SetupAssetsStatik", func(t *testing.T) {
		e := echo.New()
		err := SetupAssets(e, "")
		require.NoError(t, err)

		ts := httptest.NewServer(e)
		t.Cleanup(ts.Close)

		t.Run("WithoutHexaIsNotCached", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			e.GET("/assets/images/cozy.svg").
				Expect().Status(200).
				Header("Cache-Control").Equal("no-cache, public")
		})

		t.Run("WithHexaIsCachedAsImmutable", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			e.GET("/assets/images/cozy.badbeefbadbeef.svg").
				Expect().Status(200).
				Header("Cache-Control").
				Contains("max-age=").
				Contains("immutable")
		})

		t.Run("WithImmutableNameIsCachedAsImmutable", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			e.GET("/assets/images/cozy.immutable.svg").
				Expect().Status(200).
				Header("Cache-Control").
				Contains("max-age=").
				Contains("immutable")
		})

		t.Run("FileAvailableWithHEAD", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			e.HEAD("/assets/images/cozy.svg").Expect().Status(200)
		})

		t.Run("WithImmutableNameIsCachedAsImmutable", func(t *testing.T) {
			e := testutils.CreateTestClient(t, ts.URL)

			e.HEAD("/assets/images/cozy.badbeefbadbeef.svg").
				Expect().Status(200).
				Header("Cache-Control").
				Contains("max-age=").
				Contains("immutable")
		})
	})

	t.Run("SetupRoutes", func(t *testing.T) {
		router := echo.New()
		err := SetupRoutes(router, &stack.Services{})
		require.NoError(t, err)

		ts := httptest.NewServer(router)
		t.Cleanup(ts.Close)

		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/version").Expect().Status(200)
	})

	t.Run("ParseHost", func(t *testing.T) {
		apis := echo.New()

		apis.GET("/test", func(c echo.Context) error {
			instance := middlewares.GetInstance(c)
			assert.NotNil(t, instance, "the instance should have been set in the echo context")
			return c.String(http.StatusOK, "OK")
		}, middlewares.NeedInstance)

		router, err := CreateSubdomainProxy(apis, &stack.Services{}, func(c echo.Context) error {
			slug := c.Get("slug").(string)
			return c.String(200, "OK:"+slug)
		})
		require.NoError(t, err)

		ts := httptest.NewServer(router)
		t.Cleanup(ts.Close)

		urls := map[string]string{
			"https://" + domain + "/test":    "OK",
			"https://foo." + domain + "/app": "OK:foo",
			"https://bar." + domain + "/app": "OK:bar",
		}

		for u, k := range urls {
			t.Run(u, func(t *testing.T) {
				e := testutils.CreateTestClient(t, ts.URL)

				e.GET(u).
					Expect().Status(200).
					Body().Equal(k)
			})
		}
	})
}
