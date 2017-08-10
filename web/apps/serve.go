package apps

import (
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/intents"
	"github.com/cozy/cozy-stack/pkg/sessions"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

// Serve is an handler for serving files from the VFS for a client-side app
func Serve(c echo.Context) error {
	method := c.Request().Method
	if method != "GET" && method != "HEAD" {
		return echo.NewHTTPError(http.StatusMethodNotAllowed, "Method %s not allowed", method)
	}
	i := middlewares.GetInstance(c)
	slug := c.Get("slug").(string)
	if len(i.RegisterToken) > 0 && slug != consts.OnboardingSlug {
		return c.Redirect(http.StatusFound, i.PageURL("/", nil))
	}
	if config.GetConfig().Subdomains == config.FlatSubdomains {
		if code := c.QueryParam("code"); code != "" {
			return tryAuthWithSessionCode(c, i, code)
		}
	}
	app, err := apps.GetWebappBySlug(i, slug)
	if err != nil {
		switch err {
		case apps.ErrNotFound:
			return echo.NewHTTPError(http.StatusNotFound, "Application not found")
		case apps.ErrInvalidSlugName:
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		default:
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}
	switch app.State() {
	case apps.Installed:
		return c.Redirect(http.StatusFound, i.PageURL("/auth/authorize/app", url.Values{
			"slug": {slug},
		}))
	case apps.Ready:
		return ServeAppFile(c, i, i.AppsFileServer(), app)
	default:
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Application is not ready")
	}
}

// handleIntent will allow iframes from another app if the current app is
// opened as an intent
func handleIntent(c echo.Context, i *instance.Instance, slug, intentID string) {
	intent := &intents.Intent{}
	if err := couchdb.GetDoc(i, consts.Intents, intentID, intent); err != nil {
		return
	}
	allowed := false
	for _, service := range intent.Services {
		if slug == service.Slug {
			allowed = true
		}
	}
	if !allowed {
		return
	}
	parts := strings.SplitN(intent.Client, "/", 2)
	if len(parts) < 2 || parts[0] != consts.Apps {
		return
	}
	from := i.SubDomain(parts[1]).String()
	hdr := fmt.Sprintf("%s %s", middlewares.XFrameAllowFrom, from)
	c.Response().Header().Set(echo.HeaderXFrameOptions, hdr)
}

// ServeAppFile will serve the requested file using the specified application
// manifest and apps.FileServer context.
//
// It can be used to serve file application in another context than the VFS,
// for instance for tests or development puposes where we want to serve an
// application that is not installed on the user's instance. However this
// procedure should not be used for standard applications, use the Serve method
// for that.
func ServeAppFile(c echo.Context, i *instance.Instance, fs apps.FileServer, app *apps.WebappManifest) error {
	slug := app.Slug()
	route, file := app.FindRoute(path.Clean(c.Request().URL.Path))
	if route.NotFound() {
		return echo.NewHTTPError(http.StatusNotFound, "Page not found")
	}
	if file == "" {
		file = route.Index
	}

	var needAuth bool
	if len(i.RegisterToken) > 0 && file == route.Index {
		if slug != consts.OnboardingSlug || !permissions.CheckRegisterToken(c, i) {
			return c.Redirect(http.StatusFound, i.PageURL("/", nil))
		}
		needAuth = false
	} else {
		needAuth = !route.Public
	}

	if needAuth && !middlewares.IsLoggedIn(c) {
		if file != route.Index {
			return echo.NewHTTPError(http.StatusUnauthorized, "You must be authenticated")
		}
		reqURL := c.Request().URL
		subdomain := i.SubDomain(slug)
		subdomain.Path = reqURL.Path
		subdomain.RawQuery = reqURL.RawQuery
		subdomain.Fragment = reqURL.Fragment
		redirect := url.Values{
			"redirect": {subdomain.String()},
		}
		return c.Redirect(http.StatusFound, i.PageURL("/auth/login", redirect))
	}
	filepath := path.Join(route.Folder, file)
	version := app.Version()
	if file != route.Index {
		err := fs.ServeFileContent(c.Response(), c.Request(), slug, version, filepath)
		if os.IsNotExist(err) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err)
		}
		return nil
	}
	if intentID := c.QueryParam("intent"); intentID != "" {
		handleIntent(c, i, slug, intentID)
	}
	// For index file, we inject the locale, the stack domain, and a token if the
	// user is connected
	content, err := fs.Open(slug, version, filepath)
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
		i.Logger().Warnf("[apps] %s cannot be parsed as a template: %s", file, err)
		return fs.ServeFileContent(c.Response(), c.Request(), slug, version, filepath)
	}
	var token string
	if middlewares.IsLoggedIn(c) {
		token = i.BuildAppToken(app)
	} else {
		token = c.QueryParam("sharecode")
	}
	tracking := "false"
	settings, err := i.SettingsDocument()
	if err == nil {
		if t, ok := settings.M["tracking"].(string); ok {
			tracking = t
		}
	}
	res := c.Response()
	res.Header().Set("Content-Type", "text/html; charset=utf-8")
	res.WriteHeader(http.StatusOK)
	return tmpl.Execute(res, echo.Map{
		"Token":        token,
		"Domain":       i.Domain,
		"Locale":       i.Locale,
		"AppName":      app.Name,
		"AppEditor":    app.Editor,
		"IconPath":     app.Icon,
		"CozyBar":      cozybar(i),
		"CozyClientJS": cozyclientjs(i),
		"Tracking":     tracking,
	})
}

func tryAuthWithSessionCode(c echo.Context, i *instance.Instance, value string) error {
	u := c.Request().URL
	u.Scheme = i.Scheme()
	u.Host = c.Request().Host
	if !middlewares.IsLoggedIn(c) {
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
	}
	q := u.Query()
	q.Del("code")
	u.RawQuery = q.Encode()
	return c.Redirect(http.StatusFound, u.String())
}

var clientTemplate = template.Must(template.New("cozy-client-js").Parse(`` +
	`<script defer src="//{{.Domain}}/assets/js/cozy-client.min.js"></script>`,
))

var barTemplate = template.Must(template.New("cozy-bar").Parse(`` +
	`<link rel="stylesheet" type="text/css" href="//{{.Domain}}/assets/css/cozy-bar.min.css">` +
	`<script defer src="//{{.Domain}}/assets/js/cozy-bar.min.js"></script>`,
))

func cozyclientjs(i *instance.Instance) template.HTML {
	buf := new(bytes.Buffer)
	err := clientTemplate.Execute(buf, echo.Map{"Domain": i.Domain})
	if err != nil {
		return template.HTML("")
	}
	return template.HTML(buf.String()) // #nosec
}

func cozybar(i *instance.Instance) template.HTML {
	buf := new(bytes.Buffer)
	err := barTemplate.Execute(buf, echo.Map{"Domain": i.Domain})
	if err != nil {
		return template.HTML("")
	}
	return template.HTML(buf.String()) // #nosec
}
