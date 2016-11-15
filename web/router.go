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
	"github.com/cozy/cozy-stack/web/apps"
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

	appsRouter := gin.New()
	appsRouter.HEAD("/*anypath", middlewares.NeedInstance(), apps.Serve)
	appsRouter.GET("/*anypath", middlewares.NeedInstance(), apps.Serve)
	router.Use(middlewares.ServeApp(gin.WrapH(appsRouter)))

	router.Use(middlewares.ErrorHandler())
	apps.Routes(router.Group("/apps", middlewares.NeedInstance()))
	data.Routes(router.Group("/data", middlewares.NeedInstance()))
	files.Routes(router.Group("/files", middlewares.NeedInstance()))
	status.Routes(router.Group("/status"))
	version.Routes(router.Group("/version"))
}
