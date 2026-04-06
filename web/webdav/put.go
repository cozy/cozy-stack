package webdav

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// handlePut handles HTTP PUT requests for file creation and overwrite.
// Stub: returns 501 Not Implemented (RED state — tests must fail).
func handlePut(c echo.Context) error {
	return sendWebDAVError(c, http.StatusNotImplemented, "not-implemented")
}
