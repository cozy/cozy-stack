package status

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func testRequest(t *testing.T, url string) {
	res, err := http.Get(url)
	assert.NoError(t, err)
	defer res.Body.Close()

	body, ioerr := ioutil.ReadAll(res.Body)
	assert.NoError(t, ioerr)
	assert.Equal(t, "200 OK", res.Status, "should get a 200")
	assert.Equal(t, "{\"couchdb\":\"healthy\",\"message\":\"OK\"}\n", string(body), "res body should match")
}

func TestRoutes(t *testing.T) {
	router := gin.New()
	Routes(router.Group("/status"))

	ts := httptest.NewServer(router)
	defer ts.Close()

	testRequest(t, ts.URL+"/status")
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}
