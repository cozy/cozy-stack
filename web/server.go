// Package web Cozy Stack API.
//
// Cozy is a personal platform as a service with a focus on data.
package web

import (
	"fmt"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/web/apps"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/routing"
	"github.com/labstack/echo"
)

// Create returns a new web server that will handle that apps routing given the
// host of the request.
func Create(router *echo.Echo, serveApps echo.HandlerFunc) (*echo.Echo, error) {
	if err := routing.SetupAssets(router, config.GetConfig().Assets); err != nil {
		return nil, err
	}

	if err := routing.SetupRoutes(router); err != nil {
		return nil, err
	}

	serveApps = routing.SetupAppsHandler(serveApps)

	main := echo.New()
	main.Any("/*", func(c echo.Context) error {
		// TODO(optim): minimize the number of instance requests
		if parent, slug := middlewares.SplitHost(c.Request().Host); slug != "" {
			if i, err := instance.Get(parent); err == nil {
				c.Set("instance", i)
				c.Set("slug", slug)
				return serveApps(c)
			}
		}

		router.ServeHTTP(c.Response(), c.Request())
		return nil
	})

	main.HTTPErrorHandler = errors.ErrorHandler
	return main, nil
}

// ListenAndServe creates and setups all the necessary http endpoints and start
// them.
func ListenAndServe() error {
	main, err := Create(echo.New(), apps.Serve)
	if err != nil {
		return err
	}

	admin := echo.New()
	if err = routing.SetupAdminRoutes(admin); err != nil {
		return err
	}

	if config.IsDevRelease() {
		fmt.Println(`                           !! DEVELOPMENT RELEASE !!
You are running a development release which may deactivate some very important
security features. Please do not use this binary as your production server.
`)
	}

	errs := make(chan error)
	go func() { errs <- admin.Start(config.AdminServerAddr()) }()
	go func() { errs <- main.Start(config.ServerAddr()) }()
	return <-errs
}
