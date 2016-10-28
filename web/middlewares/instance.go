// Package middlewares is a group of functions. They mutualize some actions
// common to many gin handlers, like checking authentication or permissions.
package middlewares

import (
	"github.com/cozy/cozy-stack/instance"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/gin-gonic/gin"
)

// SetInstance creates a gin middleware to put the instance in the gin context
// for next handlers
func SetInstance() gin.HandlerFunc {
	return func(c *gin.Context) {
		i, err := instance.Get(c.Request.Host)
		if err != nil {
			jsonapi.AbortWithError(c, jsonapi.InternalServerError(err))
			return
		}
		c.Set("instance", i)
	}
}

// GetInstance will return the instance linked to the given gin
// context or panic if none exists
func GetInstance(c *gin.Context) *instance.Instance {
	return c.MustGet("instance").(*instance.Instance)
}
