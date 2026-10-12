package tools

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parkedGoroutine(started chan<- struct{}, done <-chan struct{}) {
	close(started)
	<-done
}

func TestGoroutineProfiling(t *testing.T) {
	started := make(chan struct{})
	done := make(chan struct{})
	defer close(done)
	go parkedGoroutine(started, done)
	<-started

	e := echo.New()
	Routes(e.Group("/tools"))

	req := httptest.NewRequest(http.MethodGet, "/tools/pprof/goroutine", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, echo.MIMEOctetStream, rec.Header().Get(echo.HeaderContentType))
	assert.Equal(t, `attachment; filename="goroutine.pprof"`,
		rec.Header().Get(echo.HeaderContentDisposition))

	// The pprof format is a gzipped protobuf, where the function names are
	// kept as plain strings.
	gz, err := gzip.NewReader(rec.Body)
	require.NoError(t, err)
	profile, err := io.ReadAll(gz)
	require.NoError(t, err)
	assert.Contains(t, string(profile), "web/tools.parkedGoroutine")
}
