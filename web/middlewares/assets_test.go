package middlewares_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/statik/fs"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web"
	"github.com/cozy/cozy-stack/web/apps"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
	"github.com/stretchr/testify/assert"
)

var ts *httptest.Server
var testInstance *instance.Instance
var client *http.Client

func TestTheme(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/auth/login", nil)
	req.Host = testInstance.Domain
	res, _ := client.Do(req)
	body, _ := ioutil.ReadAll(res.Body)

	assert.Equal(t, 200, res.StatusCode)
	assert.Contains(t, string(body), "/assets/styles/theme.css")

	req2, _ := http.NewRequest("GET", ts.URL+"/assets/styles/theme.css", nil)
	req2.Host = testInstance.Domain
	res2, _ := client.Do(req2)
	assert.Equal(t, 200, res2.StatusCode)
	body2, _ := ioutil.ReadAll(res2.Body)
	assert.Contains(t, string(body2), "Empty theme")
}

func TestThemeWithContext(t *testing.T) {
	context := "foo"

	asset, ok := fs.Get("/theme.css", context)
	if ok {
		fs.DeleteAsset(asset)
	}
	// Create and insert an asset in foo context
	tmpdir := os.TempDir()
	_, err := os.OpenFile(filepath.Join(tmpdir, "custom_theme.css"), os.O_RDWR|os.O_CREATE, 0600)
	assert.NoError(t, err)

	cacheStorage := config.GetConfig().CacheStorage
	assetsOptions := []fs.AssetOption{{
		URL:     fmt.Sprintf("file://%s", filepath.Join(tmpdir, "custom_theme.css")),
		Name:    "/styles/theme.css",
		Context: context,
	}}
	err = fs.RegisterCustomExternals(cacheStorage, assetsOptions, 1)
	assert.NoError(t, err)
	// Test the theme
	_ = lifecycle.Patch(testInstance, &lifecycle.Options{
		ContextName: context,
	})
	assert.NoError(t, err)
	req, _ := http.NewRequest("GET", ts.URL+"/auth/login", nil)
	req.Host = testInstance.Domain
	res, _ := client.Do(req)
	body, _ := ioutil.ReadAll(res.Body)

	assert.Equal(t, 200, res.StatusCode)
	assert.Contains(t, string(body), fmt.Sprintf("/assets/ext/%s/styles/theme.css", context))
	assert.NotContains(t, string(body), "/assets/styles/theme.css")
}

func fakeAPI(g *echo.Group) {
	g.Use(middlewares.NeedInstance, middlewares.LoadSession)
	g.GET("", func(c echo.Context) error {
		return c.String(http.StatusOK, "OK")
	})
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	config.GetConfig().Assets = "../../assets"
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "middlewares_test")

	testInstance = setup.GetTestInstance(&lifecycle.Options{
		Domain:     "middlewares.cozy.test",
		Passphrase: "MyPassphrase",
	})
	ts = setup.GetTestServer("/test", fakeAPI, func(r *echo.Echo) *echo.Echo {
		handler, err := web.CreateSubdomainProxy(r, apps.Serve)
		if err != nil {
			setup.CleanupAndDie("Cant start subdomain proxy", err)
		}
		return handler
	})

	jar := setup.GetCookieJar()
	client = &http.Client{Jar: jar}

	os.Exit(setup.Run())
}
