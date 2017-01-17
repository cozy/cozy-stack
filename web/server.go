// Package web Cozy Stack API.
//
// Cozy is a personal platform as a service with a focus on data.
package web

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

// Create returns a new web server that will handle that apps routing given the
// host of the request.
func Create(router *echo.Echo, serveApps echo.HandlerFunc) (*echo.Echo, error) {
	appsHandler := middlewares.Compose(serveApps,
		middlewares.Secure(&middlewares.SecureConfig{
			HSTSMaxAge:    365 * 24 * time.Hour, // 1 year
			CSPDefaultSrc: []middlewares.CSPSource{middlewares.CSPSrcSelf, middlewares.CSPSrcParent},
			CSPFrameSrc:   []middlewares.CSPSource{middlewares.CSPSrcParent},
			XFrameOptions: middlewares.XFrameDeny,
		}),
		middlewares.LoadSession)

	main := echo.New()
	main.Any("/*", func(c echo.Context) error {
		// TODO(optim): minimize the number of instance requests
		if parent, slug := middlewares.SplitHost(c.Request().Host); slug != "" {
			if i, err := instance.Get(parent); err == nil {
				c.Set("instance", i)
				c.Set("slug", slug)
				return appsHandler(c)
			}
		}

		router.ServeHTTP(c.Response(), c.Request())
		return nil
	})

	main.HTTPErrorHandler = errors.ErrorHandler
	return main, nil
}
