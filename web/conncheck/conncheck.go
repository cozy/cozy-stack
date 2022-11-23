// Package conncheck returns HTTP 204 No Content for connectivity check
package conncheck

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// NoContent responds with HTTP 204 No Content
func NoContent(c echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}

// Routes sets the routing for the conncheck service
func Routes(router *echo.Group) {
	router.GET("", NoContent)
	router.HEAD("", NoContent)
}
