package middlewares

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

var ins *instance.Instance

type stupidRenderer struct{}

func (sr *stupidRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return nil
}
func TestUserAgent(t *testing.T) {
	// middleware instance
	e := echo.New()

	e.Renderer = &stupidRenderer{}

	req, _ := http.NewRequest(echo.GET, "http://cozy.local", nil)
	req.Header.Set(echo.HeaderAccept, "text/html")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:63.0) Gecko/20100101 Firefox/63.0") // Firefox

	ins = &instance.Instance{Domain: "cozy.local", Locale: "en"}

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("instance", ins)

	h := CheckUserAgent(echo.NotFoundHandler)
	err := h(c)
	assert.Error(t, err)

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; WOW64; Trident/7.0; rv:11.0) like Gecko") // IE 11
	h2 := CheckUserAgent(echo.NotFoundHandler)
	err2 := h2(c)
	assert.NoError(t, err2, nil)

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML like Gecko) Chrome/51.0.2704.79 Safari/537.36 Edge/16.14931") // Edge 16
	h3 := CheckUserAgent(echo.NotFoundHandler)
	err3 := h3(c)
	assert.NoError(t, err3, nil)

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML like Gecko) Chrome/51.0.2704.79 Safari/537.36 Edge/17.14931") // Edge 17
	h4 := CheckUserAgent(echo.NotFoundHandler)
	err4 := h4(c)
	assert.Error(t, err4, nil)

	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU OS 13_3_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) FxiOS/24.1  Mobile/15E148 Safari/605.1.15") // Firefox for iOS
	h5 := CheckUserAgent(echo.NotFoundHandler)
	err5 := h5(c)
	assert.Error(t, err5, nil)
}

func TestParseEdgeVersion(t *testing.T) {
	version := "15.123456"
	v, ok := getMajorVersion(version)
	assert.Equal(t, 15, v)
	assert.Equal(t, true, ok)

	version = "75.123456.6789"
	v, ok = getMajorVersion(version)
	assert.Equal(t, 75, v)
	assert.Equal(t, true, ok)

	version = "12"
	v, ok = getMajorVersion(version)
	assert.Equal(t, 12, v)
	assert.Equal(t, true, ok)

	version = "foobar"
	v, ok = getMajorVersion(version)
	assert.Equal(t, -1, v)
	assert.Equal(t, false, ok)
}
