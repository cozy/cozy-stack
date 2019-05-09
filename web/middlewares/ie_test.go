package middlewares

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/echo"
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

	h := CheckIE(echo.NotFoundHandler)
	err := h(c)
	assert.Error(t, err)

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; WOW64; Trident/7.0; rv:11.0) like Gecko") // IE 11
	h2 := CheckIE(echo.NotFoundHandler)
	err2 := h2(c)
	assert.NoError(t, err2, nil)
}
