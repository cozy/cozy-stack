package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/instance"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

const domain = "cozy.example.net"

func TestParseHost(t *testing.T) {
	apis := echo.New()

	apis.GET("/", func(c echo.Context) error {
		instance := middlewares.GetInstance(c)
		assert.NotNil(t, instance, "the instance should have been set in the gin context")
		return c.String(http.StatusOK, "OK")
	}, middlewares.NeedInstance)

	router, err := Create(&Config{
		Router: apis,
		ServeApps: func(c echo.Context, domain, slug string) error {
			return c.String(200, "OK:"+slug)
		},
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
	instance.Create(domain, "en", nil)
	res := m.Run()
	instance.Destroy(domain)
	os.Exit(res)
}
