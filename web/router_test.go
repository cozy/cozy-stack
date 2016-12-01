package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func doReq(ts *httptest.Server, method, path string) (*http.Response, error) {
	req, err := http.NewRequest(method, ts.URL+path, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Origin", "cozy.local")

	if method == "OPTIONS" {
		req.Header.Set("Access-Control-Request-Headers", "Content-Type, X-Cozy")
	}

	return http.DefaultClient.Do(req)
}

func TestCORSMiddleware(t *testing.T) {
	router := gin.New()
	router.Use(corsMiddleware("/foo", "/bar"))

	router.GET("/foo", func(c *gin.Context) { c.AbortWithStatus(http.StatusOK) })
	router.GET("/bar", func(c *gin.Context) { c.AbortWithStatus(http.StatusOK) })
	router.GET("/quz", func(c *gin.Context) { c.AbortWithStatus(http.StatusOK) })

	ts := httptest.NewServer(router)
	defer ts.Close()

	res1, err := doReq(ts, "GET", "/quz")
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Equal(t, http.StatusForbidden, res1.StatusCode, "Should have been Forbidden") {
		return
	}
	if !assert.Equal(t, "", res1.Header.Get("Access-Control-Allow-Origin")) {
		return
	}

	res2, err := doReq(ts, "OPTIONS", "/quz")
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Equal(t, http.StatusForbidden, res2.StatusCode, "Should have been Forbidden") {
		return
	}

	res3, err := doReq(ts, "GET", "/foo")
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Equal(t, http.StatusOK, res3.StatusCode, "Should have been OK") {
		return
	}
	if !assert.Equal(t, "cozy.local", res3.Header.Get("Access-Control-Allow-Origin")) {
		return
	}

	res4, err := doReq(ts, "OPTIONS", "/foo")
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Equal(t, http.StatusNoContent, res4.StatusCode, "Should have been NoContent") {
		return
	}

	assert.Equal(t, "true", res4.Header.Get("Access-Control-Allow-Credentials"))
	assert.Equal(t, "GET,HEAD,PUT,PATCH,POST,DELETE", res4.Header.Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type, X-Cozy", res4.Header.Get("Access-Control-Allow-Headers"))
}
