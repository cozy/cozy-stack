package routing

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

var ts *httptest.Server

func TestSetupAssets(t *testing.T) {
	e := echo.New()
	err := SetupAssets(e, "../../assets")
	if !assert.NoError(t, err) {
		return
	}

	ts := httptest.NewServer(e)
	defer ts.Close()

	res, _ := http.Get(ts.URL + "/assets/images/cozy.svg")
	defer res.Body.Close()
	assert.Equal(t, 200, res.StatusCode)
}

func TestSetupAssetsStatik(t *testing.T) {
	e := echo.New()
	err := SetupAssets(e, "")
	if !assert.NoError(t, err) {
		return
	}

	ts := httptest.NewServer(e)
	defer ts.Close()

	res, _ := http.Get(ts.URL + "/assets/images/cozy.svg")
	defer res.Body.Close()
	assert.Equal(t, 200, res.StatusCode)
}

func TestSetupRoutes(t *testing.T) {
	e := echo.New()
	err := SetupRoutes(e)
	if !assert.NoError(t, err) {
		return
	}

	ts := httptest.NewServer(e)
	defer ts.Close()

	res, _ := http.Get(ts.URL + "/version")
	defer res.Body.Close()
	assert.Equal(t, 200, res.StatusCode)
}
