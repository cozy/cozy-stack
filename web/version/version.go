// Package version gives informations about the version of the cozy-stack
package version

import (
	"net/http"
	"runtime"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/echo"
)

// Version responds with the git commit used at the build
func Version(c echo.Context) error {
	return c.JSON(http.StatusOK, echo.Map{
		"version":         config.Version,
		"build_mode":      config.BuildMode,
		"build_time":      config.BuildTime,
		"runtime_version": runtime.Version(),
	})
}

// Routes sets the routing for the version service
func Routes(router *echo.Group) {
	router.GET("", Version)
	router.HEAD("", Version)
	router.GET("/", Version)
	router.HEAD("/", Version)
}
