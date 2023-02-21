// Package version gives informations about the version of the cozy-stack
package version

import (
	"net/http"
	"runtime"

	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/labstack/echo/v4"
)

// Version responds with the git commit used at the build
func Version(c echo.Context) error {
	return c.JSON(http.StatusOK, echo.Map{
		"version":         build.Version,
		"build_mode":      build.BuildMode,
		"build_time":      build.BuildTime,
		"runtime_version": runtime.Version(),
	})
}

// Routes sets the routing for the version service
func Routes(router *echo.Group) {
	router.GET("", Version)
	router.HEAD("", Version)
}
