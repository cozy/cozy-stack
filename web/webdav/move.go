package webdav

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// handleMove handles HTTP MOVE requests for renaming and reparenting files
// and directories. Per RFC 4918 section 9.9, MOVE renames the resource at
// the Request-URI to the location given in the Destination header.
//
// Stub — returns 501 Not Implemented until GREEN phase.
func handleMove(c echo.Context) error {
	return sendWebDAVError(c, http.StatusNotImplemented, "not-implemented")
}
