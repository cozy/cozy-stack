package status

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

func testRequest(t *testing.T, url string) {
	res, err := http.Get(url)
	assert.NoError(t, err)
	defer res.Body.Close()

	body, ioerr := ioutil.ReadAll(res.Body)
	assert.NoError(t, ioerr)
	assert.Equal(t, "200 OK", res.Status, "should get a 200")
	assert.Equal(t, "{\"couchdb\":\"healthy\",\"message\":\"OK\"}", string(body), "res body should match")
}

func TestRoutes(t *testing.T) {
	handler := echo.New()
	handler.HTTPErrorHandler = errors.ErrorHandler
	Routes(handler.Group("/status"))

	ts := httptest.NewServer(handler)
	defer ts.Close()

	testRequest(t, ts.URL+"/status")
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	os.Exit(m.Run())
}
