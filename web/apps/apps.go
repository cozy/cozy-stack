// Package apps is the HTTP frontend of the application package. It
// exposes the HTTP api install, update or uninstall applications.
package apps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

// JSMimeType is the content-type for javascript
const JSMimeType = "application/javascript"

const typeTextEventStream = "text/event-stream"

type apiApp struct {
	apps.Manifest
}

func (man *apiApp) MarshalJSON() ([]byte, error) {
	return json.Marshal(man.Manifest)
}

// Links is part of the Manifest interface
func (man *apiApp) Links() *jsonapi.LinksList {
	var route string
	links := jsonapi.LinksList{}
	switch app := man.Manifest.(type) {
	case (*apps.WebappManifest):
		route = "/apps/"
		if app.Icon != "" {
			links.Icon = "/apps/" + app.Slug() + "/icon"
		}
		if app.State() == apps.Ready && app.Instance != nil {
			links.Related = app.Instance.SubDomain(app.Slug()).String()
		}
	case (*apps.KonnManifest):
		route = "konnectors"
		links.Perms = "/permissions/" +
			url.QueryEscape(consts.Konnectors+"/"+app.Slug())
	}
	if route != "" {
		links.Self = route + man.Manifest.Slug()
	}
	return &links
}

// Relationships is part of the Manifest interface
func (man *apiApp) Relationships() jsonapi.RelationshipMap {
	//TODO include permissions doc
	return jsonapi.RelationshipMap{}
}

// Included is part of the Manifest interface
func (man *apiApp) Included() []jsonapi.Object {
	//TODO include permissions doc
	return []jsonapi.Object{}
}

// apiApp is a jsonapi.Object
var _ jsonapi.Object = (*apiApp)(nil)

// installHandler handles all POST /:slug request and tries to install
// or update the application with the given Source.
func installHandler(installerType apps.AppType) echo.HandlerFunc {
	return func(c echo.Context) error {
		instance := middlewares.GetInstance(c)
		slug := c.Param("slug")
		if err := permissions.AllowInstallApp(c, installerType, permissions.POST); err != nil {
			return err
		}
		var w http.ResponseWriter
		isEventStream := c.Request().Header.Get("Accept") == typeTextEventStream
		if isEventStream {
			w = c.Response().Writer
			w.Header().Set("Content-Type", typeTextEventStream)
			w.WriteHeader(200)
		}

		inst, err := apps.NewInstaller(instance, instance.AppsCopier(installerType),
			&apps.InstallerOptions{
				Operation: apps.Install,
				Type:      installerType,
				SourceURL: c.QueryParam("Source"),
				Slug:      slug,
			},
		)
		if err != nil {
			if isEventStream {
				var b []byte
				if b, err = json.Marshal(err.Error()); err == nil {
					writeStream(w, "error", string(b))
				}
			}
			return wrapAppsError(err)
		}

		go inst.Run()
		return pollInstaller(c, isEventStream, w, slug, inst)
	}
}

// updateHandler handles all POST /:slug request and tries to install
// or update the application with the given Source.
func updateHandler(installerType apps.AppType) echo.HandlerFunc {
	return func(c echo.Context) error {
		instance := middlewares.GetInstance(c)
		slug := c.Param("slug")
		if err := permissions.AllowInstallApp(c, installerType, permissions.POST); err != nil {
			return err
		}

		var w http.ResponseWriter
		isEventStream := c.Request().Header.Get("Accept") == typeTextEventStream
		if isEventStream {
			w = c.Response().Writer
			w.Header().Set("Content-Type", typeTextEventStream)
			w.WriteHeader(200)
		}

		inst, err := apps.NewInstaller(instance, instance.AppsCopier(installerType),
			&apps.InstallerOptions{
				Operation: apps.Update,
				Type:      installerType,
				SourceURL: c.QueryParam("Source"),
				Slug:      slug,
			},
		)
		if err != nil {
			if isEventStream {
				var b []byte
				if b, err = json.Marshal(err.Error()); err == nil {
					writeStream(w, "error", string(b))
				}
				return nil
			}
			return wrapAppsError(err)
		}

		go inst.Run()
		return pollInstaller(c, isEventStream, w, slug, inst)
	}
}

// deleteHandler handles all DELETE /:slug used to delete an application with
// the specified slug.
func deleteHandler(installerType apps.AppType) echo.HandlerFunc {
	return func(c echo.Context) error {
		instance := middlewares.GetInstance(c)
		slug := c.Param("slug")
		if err := permissions.AllowInstallApp(c, installerType, permissions.DELETE); err != nil {
			return err
		}
		inst, err := apps.NewInstaller(instance, instance.AppsCopier(installerType),
			&apps.InstallerOptions{
				Operation: apps.Delete,
				Type:      installerType,
				Slug:      slug,
			},
		)
		if err != nil {
			return wrapAppsError(err)
		}
		man, err := inst.RunSync()
		if err != nil {
			return wrapAppsError(err)
		}
		return jsonapi.Data(c, http.StatusOK, &apiApp{man}, nil)
	}
}

func pollInstaller(c echo.Context, isEventStream bool, w http.ResponseWriter, slug string, inst *apps.Installer) error {
	if !isEventStream {
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
		return jsonapi.Data(c, http.StatusAccepted, &apiApp{man}, nil)
	}

	for {
		man, done, err := inst.Poll()
		if err != nil {
			var b []byte
			if b, err = json.Marshal(err.Error()); err == nil {
				writeStream(w, "error", string(b))
			}
			break
		}
		buf := new(bytes.Buffer)
		if err := jsonapi.WriteData(buf, &apiApp{man}, nil); err == nil {
			writeStream(w, "state", buf.String())
		}
		if done {
			break
		}
	}
	return nil
}

func writeStream(w http.ResponseWriter, event string, b string) {
	s := fmt.Sprintf("event: %s\r\ndata: %s\r\n\r\n", event, b)
	_, err := w.Write([]byte(s))
	if err != nil {
		return
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// listWebappsHandler handles all GET / requests which can be used to list
// installed applications.
func listWebappsHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	if err := permissions.AllowWholeType(c, permissions.GET, consts.Apps); err != nil {
		return err
	}
	docs, err := apps.ListWebapps(instance)
	if err != nil {
		return wrapAppsError(err)
	}
	objs := make([]jsonapi.Object, len(docs))
	for i, d := range docs {
		d.Instance = instance
		objs[i] = &apiApp{d}
	}
	return jsonapi.DataList(c, http.StatusOK, objs, nil)
}

// listKonnectorsHandler handles all GET / requests which can be used to list
// installed applications.
func listKonnectorsHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	if err := permissions.AllowWholeType(c, permissions.GET, consts.Konnectors); err != nil {
		return err
	}
	docs, err := apps.ListKonnectors(instance)
	if err != nil {
		return wrapAppsError(err)
	}
	objs := make([]jsonapi.Object, len(docs))
	for i, d := range docs {
		objs[i] = &apiApp{d}
	}
	return jsonapi.DataList(c, http.StatusOK, objs, nil)
}

// iconHandler gives the icon of an application
func iconHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	slug := c.Param("slug")
	app, err := apps.GetWebappBySlug(instance, slug)
	if err != nil {
		return err
	}

	if err = permissions.Allow(c, permissions.GET, app); err != nil {
		return err
	}

	filepath := path.Join("/", app.Icon)
	fs := instance.AppsFileServer()
	err = fs.ServeFileContent(c.Response(), c.Request(), app.Slug(), app.Version(), filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return echo.NewHTTPError(http.StatusNotFound, err)
		}
		return err
	}
	return nil
}

// WebappsRoutes sets the routing for the web apps service
func WebappsRoutes(router *echo.Group) {
	router.GET("/", listWebappsHandler)
	router.POST("/:slug", installHandler(apps.Webapp))
	router.PUT("/:slug", updateHandler(apps.Webapp))
	router.DELETE("/:slug", deleteHandler(apps.Webapp))
	router.GET("/:slug/icon", iconHandler)
}

// KonnectorRoutes sets the routing for the konnectors service
func KonnectorRoutes(router *echo.Group) {
	router.GET("/", listKonnectorsHandler)
	router.POST("/:slug", installHandler(apps.Konnector))
	router.PUT("/:slug", updateHandler(apps.Konnector))
	router.DELETE("/:slug", deleteHandler(apps.Konnector))
}

func wrapAppsError(err error) error {
	switch err {
	case apps.ErrInvalidSlugName:
		return jsonapi.InvalidParameter("slug", err)
	case apps.ErrAlreadyExists:
		return jsonapi.Conflict(err)
	case apps.ErrNotFound:
		return jsonapi.NotFound(err)
	case apps.ErrNotSupportedSource:
		return jsonapi.InvalidParameter("Source", err)
	case apps.ErrManifestNotReachable:
		return jsonapi.NotFound(err)
	case apps.ErrSourceNotReachable:
		return jsonapi.BadRequest(err)
	case apps.ErrBadManifest:
		return jsonapi.BadRequest(err)
	case apps.ErrMissingSource:
		return jsonapi.BadRequest(err)
	}
	if _, ok := err.(*url.Error); ok {
		return jsonapi.InvalidParameter("Source", err)
	}
	return err
}
