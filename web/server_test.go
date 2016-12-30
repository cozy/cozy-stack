package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

const domain = "cozy.example.net"

func TestSplitHost(t *testing.T) {
	cfg := config.GetConfig()
	was := cfg.Subdomains
	defer func() { cfg.Subdomains = was }()

	host, app := splitHost("localhost")
	assert.Equal(t, "localhost", host)
	assert.Equal(t, "", app)

	cfg.Subdomains = config.NestedSubdomains
	host, app = splitHost("calendar.joe.example.net")
	assert.Equal(t, "joe.example.net", host)
	assert.Equal(t, "calendar", app)

	cfg.Subdomains = config.FlatSubdomains
	host, app = splitHost("joe-calendar.example.net")
	assert.Equal(t, "joe.example.net", host)
	assert.Equal(t, "calendar", app)
}

func TestParseHost(t *testing.T) {
	apis := echo.New()

	apis.GET("/", func(c echo.Context) error {
		instance := middlewares.GetInstance(c)
		assert.NotNil(t, instance, "the instance should have been set in the echo context")
		return c.String(http.StatusOK, "OK")
	}, middlewares.NeedInstance)

	router, err := Create(apis, func(c echo.Context) error {
		slug := c.Get("slug").(string)
		return c.String(200, "OK:"+slug)
	})

	if !assert.NoError(t, err) {
		return
	}

	urls := map[string]string{
		"https://" + domain + "/":        "OK",
		"https://foo." + domain + "/app": "OK:foo",
		"https://bar." + domain + "/app": "OK:bar",
	}

	for u, k := range urls {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", u, nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, k, w.Body.String())
	}
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	instance.Destroy(domain)
	instance.Create(&instance.Options{
		Domain: domain,
		Locale: "en",
	})
	res := m.Run()
	instance.Destroy(domain)
	os.Exit(res)
}
