package middlewares

import (
	"net/http"
	"strings"

	"github.com/labstack/echo"
)

// AcceptJSON is an echo middleware that checks that the HTTP Accept header
// is compatible with application/json
func AcceptJSON(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		accept := c.Request().Header.Get("Accept")
		// Drop the charset if present
		if accept != "" {
			accept = strings.SplitN(accept, ";", 2)[0]
		}
		if accept != "application/json" {
			return c.JSON(http.StatusBadRequest, echo.Map{
				"error": "bad_accept_header",
			})
		}
		return next(c)
	}
}

// ContentTypeJSON is an echo middleware that checks that the HTTP Content-Type
// header is compatible with application/json
func ContentTypeJSON(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		contentType := c.Request().Header.Get("Content-Type")
		// Drop the charset if present
		if contentType != "" {
			contentType = strings.SplitN(contentType, ";", 2)[0]
		}
		if contentType != "application/json" {
			return c.JSON(http.StatusBadRequest, echo.Map{
				"error": "bad_content_type",
			})
		}
		return next(c)
	}
}
