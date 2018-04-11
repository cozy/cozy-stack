package middlewares

import (
	"fmt"
	"time"

	"github.com/cozy/echo"
)

// CacheMode is an enum to define a cache-control mode
type CacheMode int

const (
	// NoCache is for the no-cache control mode
	NoCache CacheMode = iota + 1
	// NoStore is for the no-store control mode
	NoStore
)

// CacheOptions contains different options for the CacheControl middleware.
type CacheOptions struct {
	MaxAge         time.Duration
	Private        bool
	MustRevalidate bool
	Mode           CacheMode
}

// CacheControl returns a middleware to handle HTTP caching options.
func CacheControl(opts CacheOptions) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cache := ""
			if opts.Private {
				cache = "private"
			}
			switch opts.Mode {
			case NoCache:
				cache = appendHeader(cache, "no-cache")
			case NoStore:
				cache = appendHeader(cache, "no-store")
			}
			if opts.MustRevalidate {
				cache = appendHeader(cache, "must-revalidate")
			}
			if maxAge := opts.MaxAge; maxAge > 0 {
				cache = appendHeader(cache, fmt.Sprintf("max-age=%d", int(maxAge.Seconds())))
			}
			if cache != "" {
				c.Response().Header().Set("Cache-Control", cache)
			}
			return next(c)
		}
	}
}

func appendHeader(h, val string) string {
	if h == "" {
		return val
	}
	return h + ", " + val
}
