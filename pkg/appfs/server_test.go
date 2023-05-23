package appfs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_serveContent(t *testing.T) {
	t.Run("with other than a HEAD method", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "http://localhost/foo/bar", nil)
		w := httptest.NewRecorder()

		serveContent(w, r, "application/json", 10, strings.NewReader("foobar"))

		assert.Equal(t, http.StatusOK, w.Result().StatusCode)
		assert.Equal(t, "application/json", w.Result().Header.Get("Content-Type"))
		assert.Equal(t, "foobar", w.Body.String())

		// Not that the length 10 as the `size` parameter and not `len("foobar")`
		assert.Equal(t, "10", w.Result().Header.Get("Content-Length"))
	})

	t.Run("with the HEAD method", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodHead, "http://localhost/foo/bar", nil)
		w := httptest.NewRecorder()

		serveContent(w, r, "application/json", 10, strings.NewReader("foobar"))

		assert.Equal(t, http.StatusOK, w.Result().StatusCode)
		assert.Equal(t, "application/json", w.Result().Header.Get("Content-Type"))

		// With HEAD we don't setup a body
		assert.Empty(t, w.Body.String())

		// Not that the length 10 as the `size` parameter and not `len("foobar")`
		assert.Equal(t, "10", w.Result().Header.Get("Content-Length"))
	})
}
