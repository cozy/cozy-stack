package status

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func testRequest(t *testing.T, url string) {
	res, err := http.Get(url)
	assert.NoError(t, err)
	defer res.Body.Close()

	body, ioerr := ioutil.ReadAll(res.Body)
	assert.NoError(t, ioerr)
	assert.Equal(t, "200 OK", res.Status, "should get a 200")
	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	assert.NoError(t, err)
	assert.Equal(t, "healthy", data["cache"])
	assert.Equal(t, "healthy", data["couchdb"])
	assert.Equal(t, "healthy", data["fs"])
	assert.Equal(t, "OK", data["status"])
	assert.Equal(t, "OK", data["message"])
	assert.Contains(t, data, "latency")
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
