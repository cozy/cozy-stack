// Package apps is the HTTP frontend of the application package. It
// exposes the HTTP api install, update or uninstall applications.
package apps

import (
	"fmt"
	"net/http"
	"net/url"
	"path"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/apps"
	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/vfs"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
)

// Serve is an handler for serving files from the VFS for a client-side app
func Serve(c *gin.Context) {
	instance := middlewares.GetInstance(c)

	slug := c.MustGet("app_slug").(string)
	app, err := apps.GetBySlug(instance, slug)
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			c.AbortWithError(http.StatusNotFound, err)
		} else {
			c.AbortWithError(http.StatusInternalServerError, err)
		}
		return
	}

	if app.State != apps.Ready {
		err := fmt.Errorf("Application is not ready")
		c.AbortWithError(http.StatusServiceUnavailable, err)
		return
	}

	vpath := c.Request.URL.Path
	// TODO check path traversal attacks

	appdir := path.Join(apps.AppsDirectory, app.Slug)
	vpath = path.Join(appdir, vpath)
	doc, err := vfs.GetFileDocFromPath(instance, vpath)
	if err != nil {
		c.AbortWithError(http.StatusNotFound, err)
		return
	}

	abs, err := doc.Path(instance)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	c.Header("Content-Type", doc.Mime)
	c.File(abs)
}

func wrapAppsError(err error) *jsonapi.Error {
	if urlErr, isURLErr := err.(*url.Error); isURLErr {
		return jsonapi.InvalidParameter("Source", urlErr)
	}

	switch err {
	case apps.ErrInvalidSlugName:
		return jsonapi.InvalidParameter("slug", err)
	case apps.ErrNotSupportedSource:
		return jsonapi.InvalidParameter("Source", err)
	case apps.ErrManifestNotReachable:
		return jsonapi.NotFound(err)
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
	src := c.Query("Source")
	slug := c.Param("slug")
	inst, err := apps.NewInstaller(instance, slug, src)
	if err != nil {
		jsonapi.AbortWithError(c, wrapAppsError(err))
		return
	}

	go inst.Install()

	man, _, err := inst.WaitManifest()
	if err != nil {
		jsonapi.AbortWithError(c, wrapAppsError(err))
		return
	}

	jsonapi.Data(c, http.StatusAccepted, man, nil)

	go func() {
		for {
			_, done, err := inst.WaitManifest()
			if err != nil {
				log.Errorf("[apps] %s could not be installed: %v", slug, err)
				break
			}
			if done {
				break
			}
		}
	}()
}

// ListHandler handles all GET / requests which can be used to list
// installed applications.
func ListHandler(c *gin.Context) {
	instance := middlewares.GetInstance(c)
	docs, err := apps.List(instance)
	if err != nil {
		jsonapi.AbortWithError(c, wrapAppsError(err))
		return
	}

	objs := make([]jsonapi.Object, len(docs))
	for i, d := range docs {
		objs[i] = jsonapi.Object(d)
	}

	jsonapi.DataList(c, http.StatusOK, objs, nil)
}

// Routes sets the routing for the apps service
func Routes(router *gin.RouterGroup) {
	router.GET("/", ListHandler)
	router.POST("/:slug", InstallHandler)
}
