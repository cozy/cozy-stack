package router

import (
	"time"

	"github.com/cozy/cozy-stack/web/apps"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/data"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/settings"
	"github.com/cozy/cozy-stack/web/status"
	"github.com/cozy/cozy-stack/web/version"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
)

// Setup sets the routing for HTTP endpoints to the Go methods
func Setup(e *echo.Echo) *echo.Echo {
	cors := middleware.CORSWithConfig(middleware.CORSConfig{
		MaxAge: int(12 * time.Hour / time.Second),
	})

	auth.Routes(e.Group("", middlewares.NeedInstance))
	apps.Routes(e.Group("/apps", cors, middlewares.NeedInstance))
	data.Routes(e.Group("/data", cors, middlewares.NeedInstance))
	files.Routes(e.Group("/files", cors, middlewares.NeedInstance))
	settings.Routes(e.Group("/settings", cors, middlewares.NeedInstance))
	status.Routes(e.Group("/status", cors))
	version.Routes(e.Group("/version"))

	return e
}
