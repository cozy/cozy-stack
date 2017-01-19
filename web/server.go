// Package web Cozy Stack API.
//
// Cozy is a personal platform as a service with a focus on data.
package web

import (
	"fmt"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/web/apps"
	"github.com/labstack/echo"
)

// ListenAndServe creates and setups all the necessary http endpoints and start
// them.
func ListenAndServe() error {
	main, err := CreateSubdomainProxy(echo.New(), apps.Serve)
	if err != nil {
		return err
	}

	admin := echo.New()
	if err = SetupAdminRoutes(admin); err != nil {
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
