// Package apps is the HTTP frontend of the application package. It
// exposes the HTTP api install, update or uninstall applications.
package apps

import (
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/apps"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
)

func wrapAppsError(err error) *jsonapi.Error {
	if urlErr, isURLErr := err.(*url.Error); isURLErr {
		return jsonapi.InvalidParameter("Source", urlErr)
	}

	switch err {
	case apps.ErrInvalidSlugName:
		return jsonapi.InvalidParameter("slug", err)
	case apps.ErrNotSupportedSource:
		return jsonapi.InvalidParameter("Source", err)
	case apps.ErrSourceNotReachable:
		return jsonapi.BadRequest(err)
	case apps.ErrBadManifest:
		return jsonapi.BadRequest(err)
	}
	return jsonapi.InternalServerError(err)
}

// InstallHandler handles all POST /:slug request and tries to install
// the application with the given Source.
func InstallHandler(c *gin.Context) {
	instance := middlewares.GetInstance(c)
	vfsC, err := instance.GetVFSContext()
	if err != nil {
		jsonapi.AbortWithError(c, jsonapi.InternalServerError(err))
		return
	}

	db := instance.GetDatabasePrefix()
	src := c.Query("Source")
	slug := c.Param("slug")
	inst, err := apps.NewInstaller(vfsC, db, slug, src)
	if err != nil {
		jsonapi.AbortWithError(c, wrapAppsError(err))
		return
	}

	go inst.Install()

	man, err := inst.WaitManifest()
	if err != nil {
		jsonapi.AbortWithError(c, wrapAppsError(err))
		return
	}

	jsonapi.Data(c, http.StatusAccepted, man, nil)

	go func() {
		for {
			_, err := inst.WaitManifest()
			if err != nil {
				break
			}
			// TODO: do nothing for now
		}
	}()
}

// Routes sets the routing for the apps service
func Routes(router *gin.RouterGroup) {
	router.POST("/:slug", InstallHandler)
}
