package apps

import (
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/intents"
	"github.com/cozy/cozy-stack/pkg/sessions"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/labstack/echo"
)

// Serve is an handler for serving files from the VFS for a client-side app
func Serve(c echo.Context) error {
	method := c.Request().Method
	if method != "GET" && method != "HEAD" {
		return echo.NewHTTPError(http.StatusMethodNotAllowed, "Method "+method+" not allowed")
	}

	i := middlewares.GetInstance(c)
	slug := c.Get("slug").(string)

	if (!i.OnboardingFinished && slug != consts.OnboardingSlug && slug != consts.CollectSlug) ||
		(i.OnboardingFinished && slug == consts.OnboardingSlug) {
		return c.Redirect(http.StatusFound, i.PageURL("/", nil))
	}

	if config.GetConfig().Subdomains == config.FlatSubdomains {
		if code := c.QueryParam("code"); code != "" {
			return tryAuthWithSessionCode(c, i, code, slug)
		}
		if disconnect := c.QueryParam("disconnect"); disconnect == "true" || disconnect == "1" {
			return deleteAppCookie(c, i, slug)
		}
	}

	app, err := apps.GetWebappBySlug(i, slug)
	if err != nil {
		switch err {
		case apps.ErrNotFound:
			return echo.NewHTTPError(http.StatusNotFound, err)
		case apps.ErrInvalidSlugName:
			return echo.NewHTTPError(http.StatusBadRequest, err)
		default:
			return echo.NewHTTPError(http.StatusInternalServerError, err)
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
	} else if slug == consts.OnboardingSlug && file == route.Index {
		needAuth = true
	} else {
		needAuth = !route.Public
	}

	session, isLoggedIn := middlewares.GetSession(c)
	if needAuth && !isLoggedIn {
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

	filepath := path.Join("/", route.Folder, file)
	version := app.Version()

	if file != route.Index {
		// If file is not the index, it is considered an asset of the application
		// (JS, image, ...). For theses assets we check if it contains an unique
		// identifier to help caching. In such case, a long cache (1 year) is set.
		//
		// A unique identifier is matched when the file base contains a "long"
		// hexadecimal subpart between '.', of at least 10 characters: for instance
		// "app.badf00dbadf00d.js".
		if _, id := statik.ExtractAssetID(file); id != "" {
			c.Response().Header().Set("Cache-Control", "max-age=31536000, immutable")
		}
		return fs.ServeFileContent(c.Response(), c.Request(), slug, version, filepath)
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
	if isLoggedIn {
		token = i.BuildAppToken(app, session.ID())
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
	res.Header().Set("Cache-Control", "private, no-store, must-revalidate")
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

func tryAuthWithSessionCode(c echo.Context, i *instance.Instance, value, slug string) error {
	u := *(c.Request().URL)
	u.Scheme = i.Scheme()
	u.Host = c.Request().Host
	if code := sessions.FindCode(value, u.Host); code != nil {
		session, err := sessions.Get(i, code.SessionID)
		if err == nil {
			cookie, err := session.ToAppCookie(u.Host, slug)
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

func deleteAppCookie(c echo.Context, i *instance.Instance, slug string) error {
	c.SetCookie(&http.Cookie{
		Name:   sessions.SessionCookieName,
		Value:  "",
		MaxAge: -1,
		Path:   "/",
		Domain: utils.StripPort(i.Domain),
	})

	redirect := *(c.Request().URL)
	redirect.Scheme = i.Scheme()
	redirect.Host = c.Request().Host

	queries := make(url.Values)
	for k, v := range redirect.Query() {
		if k != "disconnect" {
			queries[k] = v
		}
	}
	redirect.RawQuery = queries.Encode()

	u := i.PageURL("/auth/login", url.Values{
		"redirect": {redirect.String()},
	})
	return c.Redirect(http.StatusFound, u)
}

var assetHelper func(domain, file string) string
var clientTemplate *template.Template
var barTemplate *template.Template

func init() {
	h := statik.NewHandler(statik.Options{Prefix: "/assets"})
	assetHelper = func(domain, file string) string {
		file = h.AssetPath(file)
		if domain != "" {
			return "//" + domain + file
		}
		return file
	}

	funcsMap := template.FuncMap{
		"split": strings.Split,
		"asset": assetHelper,
	}

	clientTemplate = template.Must(template.New("cozy-client-js").Funcs(funcsMap).Parse(`` +
		`<script defer src="{{asset .Domain "/js/cozy-client.min.js"}}"></script>`,
	))

	barTemplate = template.Must(template.New("cozy-bar").Funcs(funcsMap).Parse(`` +
		`<link rel="stylesheet" type="text/css" href="{{asset .Domain "/fonts/fonts.css"}}">` +
		`<link rel="stylesheet" type="text/css" href="{{asset .Domain "/css/cozy-bar.min.css"}}">` +
		`<script defer src="{{asset .Domain "/js/cozy-bar.min.js"}}"></script>`,
	))
}

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
