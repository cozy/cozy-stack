package webdav

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// handleMkcol handles MKCOL requests for single-directory creation.
// Stub — returns 501 until Task 2 implements the real logic.
func handleMkcol(c echo.Context) error {
	return sendWebDAVError(c, http.StatusNotImplemented, "not-implemented")
}
