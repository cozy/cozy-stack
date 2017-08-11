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
		accept := c.Request().Header.Get(echo.HeaderAccept)
		if strings.Contains(accept, echo.MIMEApplicationJSON) {
			return next(c)
		}
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "bad_accept_header",
		})
	}
}

// ContentTypeJSON is an echo middleware that checks that the HTTP Content-Type
// header is compatible with application/json
func ContentTypeJSON(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		contentType := c.Request().Header.Get(echo.HeaderContentType)
		if strings.HasPrefix(contentType, echo.MIMEApplicationJSON) {
			return next(c)
		}
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "bad_content_type",
		})
	}
}
