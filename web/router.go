package web

import (
	"github.com/gin-gonic/gin"
	"github.com/nono/cozy-stack/web/status"
)

// SetupRoutes sets the routing for HTTP endpoints to the Go methods
func SetupRoutes(router *gin.Engine) {
	status.Routes(router.Group("/status"))
}
