// Package web Cozy Stack API.
//
// Cozy is a personal platform as a service with a focus on data.
//
// Terms Of Service:
//
// there are no TOS at this moment, use at your own risk we take no responsibility
//
//     Schemes: https
//     Host: localhost
//     BasePath: /
//     Version: 0.0.1
//     License: AGPL-3.0 https://opensource.org/licenses/agpl-3.0
//     Contact: Bruno Michel <bruno@cozycloud.cc> https://cozy.io/
//
//     Consumes:
//     - application/json
//
//     Produces:
//     - application/json
//
// swagger:meta
package web

import (
	"strings"

	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/instance"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
)

func splitHost(host string) (instanceHost string, appSlug string) {
	parts := strings.SplitN(host, ".", 2)
	if len(parts) == 2 {
		return parts[1], parts[0]
	}
	return parts[0], ""
}

// Create returns a new web server that will handle that apps routing given the
// host of the request.
func Create(router *echo.Echo, serveApps func(c echo.Context, domain, slug string) error) (*echo.Echo, error) {
	recoverMiddleware := middleware.RecoverWithConfig(middleware.RecoverConfig{
		StackSize:         1 << 10, // 1 KB
		DisableStackAll:   !config.IsDevRelease(),
		DisablePrintStack: !config.IsDevRelease(),
	})

	router.Use(recoverMiddleware)

	main := echo.New()
	main.Any("/*", func(c echo.Context) error {
		// TODO(optim): minimize the number of instance requests
		if parent, slug := splitHost(c.Request().Host); slug != "" {
			if _, err := instance.Get(parent); err == nil {
				if serveApps != nil {
					return serveApps(c, parent, slug)
				}
				return nil
			}
		}

		router.ServeHTTP(c.Response(), c.Request())
		return nil
	})

	return main, nil
}
