// Package middlewares is a group of functions. They mutualize some actions
// common to many gin handlers, like checking authentication or permissions.
package middlewares

import (
	"errors"
	"strings"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/instance"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/gin-gonic/gin"
)

func splitHost(host string) (instanceHost string, appSlug string) {
	parts := strings.SplitN(host, ".", 2)
	if len(parts) == 2 {
		return parts[1], parts[0]
	}
	return parts[0], ""
}

// ParseHost creates a gin middleware to parse the host, find the associate
// instance, and detect if we are on a sub-domain for an app
func ParseHost() gin.HandlerFunc {
	return func(c *gin.Context) {
		i, err := instance.Get(c.Request.Host)
		if err == nil {
			c.Set("instance", i)
			return
		}

		parent, slug := splitHost(c.Request.Host)
		i, errapp := instance.Get(parent)
		if errapp == nil {
			c.Set("instance", i)
			c.Set("app_slug", slug)
			return
		}

		c.Set("instance_error", err)
	}
}

// NeedInstance is a gin middleware which will display an error
// if there is no instance.
func NeedInstance() gin.HandlerFunc {
	return func(c *gin.Context) {
		errInterface, ok := c.Get("instance_error")
		if ok {
			err := errInterface.(error)
			if couchdb.IsNotFoundError(err) {
				jsonapi.AbortWithError(c, jsonapi.NotFound(err))
				return
			}
			jsonapi.AbortWithError(c, jsonapi.InternalServerError(err))
		}

		instInterface, ok := c.Get("instance")
		if !ok {
			jsonapi.AbortWithError(c, jsonapi.InternalServerError(errors.New("no instance")))
		}

		_, ok = instInterface.(*instance.Instance)
		if !ok {
			jsonapi.AbortWithError(c, jsonapi.InternalServerError(errors.New("wrong instance type")))
		}
	}
}

// GetInstance will return the instance linked to the given gin
// context or panic if none exists
func GetInstance(c *gin.Context) *instance.Instance {
	return c.MustGet("instance").(*instance.Instance)
}
