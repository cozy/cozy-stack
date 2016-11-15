package middlewares

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/instance"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

const domain = "cozy.example.net"

func TestParseHost(t *testing.T) {
	router := gin.New()
	router.Use(ParseHost())
	router.GET("/:id", func(c *gin.Context) {
		instanceInterface, exists := c.Get("instance")
		assert.True(t, exists, "the instance should have been set in the gin context")
		instance := instanceInterface.(*instance.Instance)
		assert.Equal(t, domain, instance.Domain, "the domain should have been set in the instance")
		storage := instance.FS()
		assert.NotNil(t, storage, "the instance should have a storage provider")
		slug, inApp := c.Get("app_slug")
		assert.Equal(t, c.Param("id") == "app", inApp)
		if inApp {
			assert.Equal(t, "foo", slug)
		}
		c.String(http.StatusOK, "OK")
	})

	urls := []string{
		"https://" + domain + "/stack",
		"https://foo." + domain + "/app",
	}
	for _, u := range urls {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", u, nil)
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "OK", w.Body.String())
	}
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	instance.Destroy("cozy.example.net")
	instance.Create("cozy.example.net", "en", nil)
	gin.SetMode(gin.TestMode)
	res := m.Run()
	instance.Destroy("cozy.example.net")
	os.Exit(res)
}
