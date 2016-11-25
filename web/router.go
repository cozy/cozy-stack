// Package web Cozy Stack API.
//
// Cozy is a personal platform as a service with a focus on data.
//
// Terms Of Service:
//
// there are no TOS at this moment, use at your own risk we take no responsibility
//
//     Schemes: https
//     Host: localhost
//     BasePath: /
//     Version: 0.0.1
//     License: AGPL-3.0 https://opensource.org/licenses/agpl-3.0
//     Contact: Bruno Michel <bruno@cozycloud.cc> https://cozy.io/
//
//     Consumes:
//     - application/json
//
//     Produces:
//     - application/json
//
// swagger:meta
package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/web/apps"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/data"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/status"
	"github.com/cozy/cozy-stack/web/version"
	"github.com/gin-gonic/gin"
)

// SetupRoutes sets the routing for HTTP endpoints to the Go methods
func SetupRoutes(router *gin.Engine) {
	router.Use(middlewares.ParseHost())
	router.Use(middlewares.ServeApp(apps.Serve))
	router.Use(middlewares.ErrorHandler())

	// NOTE: CORS middleware is a "global" one because of the way gin works: to
	// handle preflight requests with OPTIONS method, every route would have to
	// have an empty OPTIONS handler in order to not get a 404 and actually
	// entering the middleware.
	router.Use(corsMiddleware("/apps", "/data", "/files"))

	auth.Routes(router)
	apps.Routes(router.Group("/apps", middlewares.NeedInstance()))
	data.Routes(router.Group("/data", middlewares.NeedInstance()))
	files.Routes(router.Group("/files", middlewares.NeedInstance()))
	status.Routes(router.Group("/status"))
	version.Routes(router.Group("/version"))
}

func corsMiddleware(routes ...string) gin.HandlerFunc {
	normalHeaders := make(http.Header)
	normalHeaders.Set("Access-Control-Allow-Credentials", "true")

	preflightHeaders := make(http.Header)
	preflightHeaders.Set("Access-Control-Allow-Credentials", "true")
	preflightHeaders.Set("Access-Control-Allow-Methods", "*")
	preflightHeaders.Set("Access-Control-Max-Age", strconv.FormatInt(int64(12*time.Hour/time.Second), 10))

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// TODO(oauth): check that the Origin header matches a subdomain of the
		// instance or a registered domain for an OAuth2 app/device.
		if origin == "" {
			return
		}

		path := c.Request.URL.Path
		valid := false
		for _, route := range routes {
			if strings.HasPrefix(path, route) {
				valid = true
				break
			}
		}

		if !valid {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		isOptions := c.Request.Method == "OPTIONS"

		var headers http.Header
		if isOptions {
			headers = preflightHeaders
		} else {
			headers = normalHeaders
		}

		header := c.Writer.Header()
		for key, value := range headers {
			header[key] = value
		}

		c.Header("Access-Control-Allow-Origin", origin)

		if isOptions {
			c.Header("Access-Control-Allow-Headers", c.Request.Header.Get("Access-Control-Request-Headers"))
			c.AbortWithStatus(http.StatusNoContent)
		}
	}
}
