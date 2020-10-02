package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestCORSMiddleware(t *testing.T) {
	e := echo.New()
	req, _ := http.NewRequest(echo.OPTIONS, "http://cozy.local/data/io.cozy.files", nil)
	req.Header.Set("Origin", "fakecozy.local")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	h := CORS(CORSOptions{})(echo.NotFoundHandler)
	_ = h(c)
	assert.Equal(t, "fakecozy.local", rec.Header().Get(echo.HeaderAccessControlAllowOrigin))
}

func TestCORSMiddlewareNotAuth(t *testing.T) {
	e := echo.New()
	req, _ := http.NewRequest(echo.OPTIONS, "http://cozy.local/auth/register", nil)
	req.Header.Set("Origin", "fakecozy.local")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath(req.URL.Path)
	h := CORS(CORSOptions{BlockList: []string{"/auth/"}})(echo.NotFoundHandler)
	_ = h(c)
	assert.Equal(t, "", rec.Header().Get(echo.HeaderAccessControlAllowOrigin))
}
