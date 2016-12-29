// Package web Cozy Stack API.
//
// Cozy is a personal platform as a service with a focus on data.
package web

import (
	"strings"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

func splitHost(host string) (instanceHost string, appSlug string) {
	parts := strings.SplitN(host, ".", 2)
	if len(parts) == 2 {
		if config.GetConfig().Subdomains == config.FlatSubdomains {
			subs := strings.SplitN(parts[0], "-", 2)
			if len(subs) == 2 {
				return subs[0] + "." + parts[1], subs[1]
			}
		} else {
			return parts[1], parts[0]
		}
	}
	return parts[0], ""
}

// Create returns a new web server that will handle that apps routing given the
// host of the request.
func Create(router *echo.Echo, serveApps echo.HandlerFunc) (*echo.Echo, error) {
	appsHandler := middlewares.LoadSession(serveApps)
	main := echo.New()
	main.Any("/*", func(c echo.Context) error {
		// TODO(optim): minimize the number of instance requests
		if parent, slug := splitHost(c.Request().Host); slug != "" {
			if i, err := instance.Get(parent); err == nil {
				if serveApps != nil {
					c.Set("instance", i)
					c.Set("slug", slug)
					return appsHandler(c)
				}
				return nil
			}
		}

		router.ServeHTTP(c.Response(), c.Request())
		return nil
	})

	return main, nil
}
