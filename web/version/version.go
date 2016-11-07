// Package version gives informations about the version of the cozy-stack
package version

import (
	"net/http"
	"runtime"

	"github.com/cozy/cozy-stack/config"
	"github.com/gin-gonic/gin"
)

// Version responds with the git commit used at the build
//
// swagger:route GET /version version showVersion
//
// It responds with the git commit used at the build
func Version(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version":         config.Version,
		"build_mode":      config.BuildMode,
		"build_time":      config.BuildTime,
		"runtime_version": runtime.Version(),
	})
}

// Routes sets the routing for the version service
func Routes(router *gin.RouterGroup) {
	router.GET("/", Version)
}
