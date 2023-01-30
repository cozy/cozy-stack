package status

import (
	"net/http/httptest"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
)

func TestStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile()

	t.Run("Routes", func(t *testing.T) {
		handler := echo.New()
		handler.HTTPErrorHandler = errors.ErrorHandler
		Routes(handler.Group("/status"))

		ts := httptest.NewServer(handler)
		t.Cleanup(ts.Close)

		e := httpexpect.Default(t, ts.URL)

		obj := e.GET("/status").
			Expect().Status(200).
			JSON().Object()

		obj.ValueEqual("cache", "healthy")
		obj.ValueEqual("couchdb", "healthy")
		obj.ValueEqual("fs", "healthy")
		obj.ValueEqual("status", "OK")
		obj.ValueEqual("message", "OK")
		latencies := obj.Value("latency").Object()
		latencies.Value("cache").String().NotEmpty()
		latencies.Value("couchdb").String().NotEmpty()
		latencies.Value("fs").String().NotEmpty()
	})
}
