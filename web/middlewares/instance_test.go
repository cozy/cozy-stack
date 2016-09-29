package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestSetInstance(t *testing.T) {
	router := gin.New()
	router.Use(SetInstance())
	router.GET("/", func(c *gin.Context) {
		instanceInterface, exists := c.Get("instance")
		assert.True(t, exists, "instance should have been set in the gin context")
		instance := instanceInterface.(Instance)
		assert.Equal(t, "dev", instance.Domain, "domain should have been set in the instance")
		c.String(http.StatusOK, "OK")
	})
	ts := httptest.NewServer(router)
	defer ts.Close()
	res, err := http.Get(ts.URL + "/")
	assert.NoError(t, err)
	res.Body.Close()
}
