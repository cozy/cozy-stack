package web

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func testRequest(t *testing.T, url string) {
	resp, err := http.Get(url)
	assert.NoError(t, err)
	defer resp.Body.Close()

	body, ioerr := ioutil.ReadAll(resp.Body)
	assert.NoError(t, ioerr)
	assert.Equal(t, "200 OK", resp.Status, "should get a 200")
	assert.Equal(t, "{\"message\":\"pong\"}\n", string(body), "resp body should match")
}

func TestPingRoute(t *testing.T) {
	router := gin.New()
	SetupRoutes(router)

	ts := httptest.NewServer(router)
	defer ts.Close()

	testRequest(t, ts.URL+"/ping")
}
