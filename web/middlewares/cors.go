package middlewares

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/echo"
)

// MaxAgeCORS is used to cache the CORS header for 12 hours
const MaxAgeCORS = "43200"

// CORSOptions contains different options to create a CORS middleware.
type CORSOptions struct {
	MaxAge         time.Duration
	BlackList      []string
	AllowedMethods []string
}

// CORS returns a Cross-Origin Resource Sharing (CORS) middleware.
// See: https://developer.mozilla.org/en/docs/Web/HTTP/Access_control_CORS
func CORS(opts CORSOptions) echo.MiddlewareFunc {
	var maxAge string
	if opts.MaxAge != 0 {
		maxAge = strconv.Itoa(int(opts.MaxAge.Seconds()))
	} else {
		maxAge = MaxAgeCORS
	}

	var allowedMethods []string
	if opts.AllowedMethods == nil {
		allowedMethods = []string{
			echo.GET,
			echo.HEAD,
			echo.PUT,
			echo.PATCH,
			echo.POST,
			echo.DELETE,
		}
	}

	allowMethods := strings.Join(allowedMethods, ",")

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			res := c.Response()

			origin := req.Header.Get(echo.HeaderOrigin)
			if origin == "" {
				return next(c)
			}

			path := c.Path()
			for _, route := range opts.BlackList {
				if strings.HasPrefix(path, route) {
					return next(c)
				}
			}

			// Simple request
			if req.Method != echo.OPTIONS {
				res.Header().Add(echo.HeaderVary, echo.HeaderOrigin)
				res.Header().Set(echo.HeaderAccessControlAllowOrigin, origin)
				res.Header().Set(echo.HeaderAccessControlAllowCredentials, "true")
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

			res.Header().Set(echo.HeaderAccessControlMaxAge, maxAge)

			return c.NoContent(http.StatusNoContent)
		}
	}
}
