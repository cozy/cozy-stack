package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServers_ok(t *testing.T) {
	major := echo.New()
	major.HideBanner = true
	major.HidePort = true
	major.GET("/ping", func(e echo.Context) error {
		return e.Blob(http.StatusOK, "application/text", []byte("pong major"))
	})

	servers := NewServers()
	defer servers.Shutdown(context.Background())

	err := servers.Start(major, "major", "localhost:")
	require.NoError(t, err)

	res, err := http.Get(fmt.Sprintf("http://%s/ping", servers.GetAddr("major")))
	require.NoError(t, err)
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	assert.Equal(t, "pong major", string(raw))
}

func TestServers_handle_ipv4_ipv6_on_localhost(t *testing.T) {
	major := echo.New()
	major.HideBanner = true
	major.HidePort = true
	major.GET("/ping", func(e echo.Context) error {
		return e.Blob(http.StatusOK, "application/text", []byte("pong major"))
	})

	servers := NewServers()
	defer servers.Shutdown(context.Background())

	err := servers.Start(major, "major", "localhost:38423")
	require.NoError(t, err)

	// Need some time to start the goroutines.
	time.Sleep(50 * time.Millisecond)

	// Call the host on IPv4
	res, err := http.Get("http://127.0.0.1:38423/ping")
	require.NoError(t, err)

	raw, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	assert.Equal(t, "pong major", string(raw))

	// Call the host on IPv6
	res, err = http.Get("http://::1:38423/ping")
	require.NoError(t, err)
	defer res.Body.Close()

	raw, err = io.ReadAll(res.Body)
	require.NoError(t, err)

	assert.Equal(t, "pong major", string(raw))
}

func TestServers_with_a_missing_port(t *testing.T) {
	major := echo.New()
	major.HideBanner = true
	major.HidePort = true
	major.GET("/ping", func(e echo.Context) error {
		return e.Blob(http.StatusOK, "application/text", []byte("pong major"))
	})

	servers := NewServers()
	defer servers.Shutdown(context.Background())

	err := servers.Start(major, "major", "localhost")
	assert.EqualError(t, err, "address localhost: missing port in address")
}

func TestServers_with_an_invalid_host(t *testing.T) {
	major := echo.New()
	major.HideBanner = true
	major.HidePort = true
	major.GET("/ping", func(e echo.Context) error {
		return e.Blob(http.StatusOK, "application/text", []byte("pong major"))
	})

	servers := NewServers()
	defer servers.Shutdown(context.Background())

	err := servers.Start(major, "major", "[[localhost:32")
	assert.EqualError(t, err, "address [[localhost:32: missing ']' in address")
}

func TestServers_with_an_missing_argument(t *testing.T) {
	major := echo.New()
	major.HideBanner = true
	major.HidePort = true
	major.GET("/ping", func(e echo.Context) error {
		return e.Blob(http.StatusOK, "application/text", []byte("pong major"))
	})

	servers := NewServers()
	defer servers.Shutdown(context.Background())

	err := servers.Start(major, "", "localhost:432")
	assert.ErrorIs(t, err, ErrMissingArgument)

	err = servers.Start(major, "major", "")
	assert.ErrorIs(t, err, ErrMissingArgument)
}
