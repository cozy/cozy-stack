// Package version gives informations about the version of the cozy-stack
package version

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Build is the git commit used at compilation
// go build -ldflags "-X github.com/cozy/cozy-stack/web/version.Build=<sha1>"
var Build = "Unknown"

// Version responds the git commit used at the build
//
// swagger:route GET /version version showVersion
//
// It responds with the git commit used at the build
func Version(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"build": Build,
	})
}

// Routes sets the routing for the version service
func Routes(router *gin.RouterGroup) {
	router.GET("/", Version)
}
