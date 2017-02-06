// Package apps is the HTTP frontend of the application package. It
// exposes the HTTP api install, update or uninstall applications.
package apps

import (
	"fmt"
	"net/http"
	"net/url"
	"text/template"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

// JSMimeType is the content-type for javascript
const JSMimeType = "application/javascript"

// CozyBarJS is the JavaScript used to initialize a cozy-bar
const CozyBarJS = `document.addEventListener('DOMContentLoaded', () => {
	cozy.bar.init({
		appName: '%s',
		iconPath: '%s',
		lang: '%s'
	})
})`

// InitCozyBarJS returns a JavaScript that initializes the cozy-bar with
// options from the manifest of the application
func InitCozyBarJS(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	slug := c.Param("slug")

	man, err := apps.GetBySlug(instance, slug)
	if err != nil {
		return err
	}

	appName := template.JSEscapeString(man.Name)
	iconPath := template.JSEscapeString(man.Icon)
	lang := template.JSEscapeString(instance.Locale)
	body := fmt.Sprintf(CozyBarJS, appName, iconPath, lang)
	return c.Blob(http.StatusOK, JSMimeType, []byte(body))
}

// InstallOrUpdateHandler handles all POST /:slug request and tries to install
// or update the application with the given Source.
func InstallOrUpdateHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	slug := c.Param("slug")
	inst, err := apps.NewInstaller(instance, &apps.InstallerOptions{
		SourceURL: c.QueryParam("Source"),
		Slug:      slug,
	})
	if err != nil {
		return wrapAppsError(err)
	}

	go inst.InstallOrUpdate()

	man, _, err := inst.Poll()
	if err != nil {
		return wrapAppsError(err)
	}

	go func() {
		for {
			_, done, err := inst.Poll()
			if err != nil {
				log.Errorf("[apps] %s could not be installed: %v", slug, err)
				break
			}
			if done {
				break
			}
		}
	}()

	return jsonapi.Data(c, http.StatusAccepted, man, nil)
}

// ListHandler handles all GET / requests which can be used to list
// installed applications.
func ListHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	docs, err := apps.List(instance)
	if err != nil {
		return wrapAppsError(err)
	}

	objs := make([]jsonapi.Object, len(docs))
	for i, d := range docs {
		d.Instance = instance
		objs[i] = jsonapi.Object(d)
	}

	return jsonapi.DataList(c, http.StatusOK, objs, nil)
}

// Routes sets the routing for the apps service
func Routes(router *echo.Group) {
	router.GET("/", ListHandler)
	router.POST("/:slug", InstallOrUpdateHandler)
	router.GET("/:slug/init-cozy-bar.js", InitCozyBarJS)
}

func wrapAppsError(err error) error {
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
	if _, ok := err.(*url.Error); ok {
		return jsonapi.InvalidParameter("Source", err)
	}
	return err
}
