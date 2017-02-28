package middlewares

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo"
)

// corsBlackList list all routes prefix that are not eligible to CORS
var corsBlackList = []string{
	"/auth/",
}

// MaxAgeCORS is used to cache the CORS header for 12 hours
var MaxAgeCORS = strconv.Itoa(int(12 * time.Hour / time.Second))
var allowMethods = strings.Join([]string{echo.GET, echo.HEAD, echo.PUT, echo.PATCH, echo.POST, echo.DELETE}, ",")

// CORS returns a Cross-Origin Resource Sharing (CORS) middleware.
// See: https://developer.mozilla.org/en/docs/Web/HTTP/Access_control_CORS
func CORS(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {

		path := c.Path()
		for _, route := range corsBlackList {
			if strings.HasPrefix(path, route) {
				return next(c)
			}
		}

		req := c.Request()
		res := c.Response()

		// @TODO validate origin against oauth clients ?
		origin := req.Header.Get(echo.HeaderOrigin)

		// Simple request
		if req.Method != echo.OPTIONS {
			res.Header().Add(echo.HeaderVary, echo.HeaderOrigin)
			res.Header().Set(echo.HeaderAccessControlAllowOrigin, origin)
			res.Header().Set(echo.HeaderAccessControlAllowCredentials, "true")
			// if exposeHeaders != "" {
			// 	res.Header().Set(echo.HeaderAccessControlExposeHeaders, exposeHeaders)
			// }
			return next(c)
		}

		// Preflight request
		res.Header().Add(echo.HeaderVary, echo.HeaderOrigin)
		res.Header().Add(echo.HeaderVary, echo.HeaderAccessControlRequestMethod)
		res.Header().Add(echo.HeaderVary, echo.HeaderAccessControlRequestHeaders)
		res.Header().Set(echo.HeaderAccessControlAllowOrigin, origin)
		res.Header().Set(echo.HeaderAccessControlAllowMethods, allowMethods)
		res.Header().Set(echo.HeaderAccessControlAllowCredentials, "true")

		h := req.Header.Get(echo.HeaderAccessControlRequestHeaders)
		if h != "" {
			res.Header().Set(echo.HeaderAccessControlAllowHeaders, h)
		}

		res.Header().Set(echo.HeaderAccessControlMaxAge, MaxAgeCORS)

		return c.NoContent(http.StatusNoContent)
	}
}
