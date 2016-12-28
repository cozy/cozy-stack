// Package apps is the HTTP frontend of the application package. It
// exposes the HTTP api install, update or uninstall applications.
package apps

import (
	"html/template"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/labstack/echo"
)

func buildCtxToken(i *instance.Instance, app *apps.Manifest, ctx apps.Context) string {
	subject := "public"
	if !ctx.Public {
		subject = ctx.Folder
	}
	token, err := crypto.NewJWT(i.SessionSecret, jwt.StandardClaims{
		Audience: "context",
		Issuer:   i.SubDomain(app.Slug),
		IssuedAt: crypto.Timestamp(),
		Subject:  subject,
	})
	if err != nil {
		return ""
	}
	return token
}

func serveApp(c echo.Context, i *instance.Instance, app *apps.Manifest, vpath string) error {
	ctx, file := app.FindContext(vpath)
	if ctx.NotFound() {
		return echo.NewHTTPError(http.StatusNotFound, "Page not found")
	}
	if !ctx.Public && !middlewares.IsLoggedIn(c) {
		return echo.NewHTTPError(http.StatusUnauthorized, "You must be authenticated")
	}
	if file == "" {
		file = ctx.Index
	}
	filepath := path.Join(vfs.AppsDirName, app.Slug, ctx.Folder, file)
	doc, err := vfs.GetFileDocFromPath(i, filepath)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	res := c.Response()
	if file != ctx.Index {
		return vfs.ServeFileContent(i, doc, "", c.Request(), res)
	}

	// For index file, we inject the stack domain and a context token
	name, err := doc.Path(i)
	if err != nil {
		return err
	}
	content, err := i.FS().Open(name)
	if err != nil {
		return err
	}
	defer content.Close()
	buf, err := ioutil.ReadAll(content)
	if err != nil {
		return err
	}
	tmpl, err := template.New(file).Parse(string(buf))
	if err != nil {
		log.Printf("%s cannot be parsed as a template: %s", vpath, err)
		return vfs.ServeFileContent(i, doc, "", c.Request(), c.Response())
	}
	res.Header().Set("Content-Type", doc.Mime)
	res.WriteHeader(http.StatusOK)
	return tmpl.Execute(res, echo.Map{
		"CtxToken": buildCtxToken(i, app, ctx),
		"Domain":   i.Domain,
	})
}

// Serve is an handler for serving files from the VFS for a client-side app
func Serve(c echo.Context) error {
	req := c.Request()
	if req.Method != "GET" && req.Method != "HEAD" {
		return echo.NewHTTPError(http.StatusMethodNotAllowed, "Method %s not allowed", req.Method)
	}

	slug := c.Get("slug").(string)
	i := middlewares.GetInstance(c)
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
