package web

import (
	"github.com/gin-gonic/gin"
)

// SetupRoutes sets the routing for HTTP endpoints to the Go methods
func SetupRoutes(router *gin.Engine) {
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})
}
