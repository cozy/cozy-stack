package apps

import (
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"path"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/sessions"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

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

func tryAuthWithSessionCode(c echo.Context, i *instance.Instance, value string) error {
	u := c.Request().URL
	u.Scheme = i.Scheme()
	u.Host = c.Request().Host
	if code := sessions.FindCode(value, u.Host); code != nil {
		var session sessions.Session
		err := couchdb.GetDoc(i, consts.Sessions, code.SessionID, &session)
		if err == nil {
			session.Instance = i
			cookie, err := session.ToAppCookie(u.Host)
			if err == nil {
				c.SetCookie(cookie)
			}
		}
	}
	q := u.Query()
	q.Del("code")
	u.RawQuery = q.Encode()
	return c.Redirect(http.StatusFound, u.String())
}

func serveApp(c echo.Context, i *instance.Instance, app *apps.Manifest, vpath string) error {
	route, file := app.FindRoute(vpath)
	cfg := config.GetConfig()
	if cfg.Subdomains == config.FlatSubdomains && !middlewares.IsLoggedIn(c) {
		if code := c.QueryParam("code"); code != "" {
			return tryAuthWithSessionCode(c, i, code)
		}
	}
	if route.NotFound() {
		return echo.NewHTTPError(http.StatusNotFound, "Page not found")
	}
	if !route.Public && !middlewares.IsLoggedIn(c) {
		if file != "" {
			return echo.NewHTTPError(http.StatusUnauthorized, "You must be authenticated")
		}
		redirect := url.Values{
			"redirect": {i.SubDomain(app.Slug) + c.Request().URL.String()},
		}
		return c.Redirect(http.StatusFound, i.PageURL("/auth/login", redirect))
	}
	if file == "" {
		file = route.Index
	}
	filepath := path.Join(vfs.AppsDirName, app.Slug, route.Folder, file)
	doc, err := vfs.GetFileDocFromPath(i, filepath)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	res := c.Response()
	if file != route.Index {
		return vfs.ServeFileContent(i, doc, "", c.Request(), res)
	}

	// For index file, we inject the locale, the stack domain, and a token if the user is connected
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
		log.Printf("[apps] %s cannot be parsed as a template: %s", vpath, err)
		return vfs.ServeFileContent(i, doc, "", c.Request(), c.Response())
	}
	token := "" // #nosec
	if middlewares.IsLoggedIn(c) {
		token = app.BuildToken(i)
	}
	res.Header().Set("Content-Type", doc.Mime)
	res.WriteHeader(http.StatusOK)
	return tmpl.Execute(res, echo.Map{
		"Token":  token,
		"Domain": i.Domain,
		"Locale": i.Locale,
	})
}
