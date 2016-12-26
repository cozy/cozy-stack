// Package apps is the HTTP frontend of the application package. It
// exposes the HTTP api install, update or uninstall applications.
package apps

import (
	"net/http"
	"net/url"
	"path"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

const indexPage = "index.html"

func serveApp(c echo.Context, i *instance.Instance, app *apps.Manifest, vpath string) error {
	ctx, file := app.FindContext(vpath)
	if ctx.Folder == "" {
		return echo.NewHTTPError(http.StatusNotFound, "Page not found")
	}
	if file == "" {
		file = ctx.Index
	}
	appdir := path.Join(vfs.AppsDirName, app.Slug)
	filepath := path.Join(appdir, ctx.Folder, file)
	doc, err := vfs.GetFileDocFromPath(i, filepath)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound)
	}

	vfs.ServeFileContent(i, doc, "", c.Request(), c.Response())
	return nil
}

// Serve is an handler for serving files from the VFS for a client-side app
func Serve(c echo.Context, domain, slug string) error {
	req := c.Request()
	if req.Method != "GET" && req.Method != "HEAD" {
		return echo.NewHTTPError(http.StatusMethodNotAllowed, "Method %s not allowed", req.Method)
	}

	i, err := instance.Get(domain)
	if err != nil {
		return err
	}
	app, err := apps.GetBySlug(i, slug)
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			return echo.NewHTTPError(http.StatusNotFound, "Application not found")
		}
		return err
	}
	if app.State != apps.Ready {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Application is not ready")
	}

	return serveApp(c, i, app, path.Clean(req.URL.Path))
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

	jsonapi.Data(c, http.StatusAccepted, man, nil)

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

	return nil
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
		objs[i] = jsonapi.Object(d)
	}

	return jsonapi.DataList(c, http.StatusOK, objs, nil)
}

// Routes sets the routing for the apps service
func Routes(router *echo.Group) {
	router.GET("/", ListHandler)
	router.POST("/:slug", InstallOrUpdateHandler)
}
