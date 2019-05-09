package apps

import (
	"bytes"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/intent"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/session"
	"github.com/cozy/cozy-stack/pkg/appfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	statikfs "github.com/cozy/cozy-stack/pkg/statik/fs"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/cozy/echo"
)

// Serve is an handler for serving files from the VFS for a client-side app
func Serve(c echo.Context) error {
	method := c.Request().Method
	if method != "GET" && method != "HEAD" {
		return echo.NewHTTPError(http.StatusMethodNotAllowed, "Method "+method+" not allowed")
	}

	i := middlewares.GetInstance(c)
	slug := c.Get("slug").(string)

	if !i.OnboardingFinished {
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

	webapp, err := app.GetWebappBySlug(i, slug)
	if err != nil {
		// Used for the "collect" => "home" renaming
		if err == app.ErrNotFound && slug == "collect" {
			return c.Redirect(http.StatusMovedPermanently, i.DefaultRedirection().String())
		}
		// Used for the deprecated "onboarding" app
		if err == app.ErrNotFound && slug == "onboarding" {
			return c.Redirect(http.StatusMovedPermanently, i.DefaultRedirection().String())
		}
		return err
	}

	route, file := webapp.FindRoute(path.Clean(c.Request().URL.Path))

	if webapp.FromAppsDir {
		// Save permissions in couchdb before loading an index page
		if file == "" && webapp.Permissions() != nil {
			err := permission.ForceWebapp(i, webapp.Slug(), webapp.Permissions())
			if err != nil {
				return err
			}
		}

		fs := app.FSForAppDir(slug)
		f := appfs.NewAferoFileServer(fs, func(_, _, _, file string) string {
			return path.Join("/", file)
		})
		return ServeAppFile(c, i, f, webapp)
	}

	if file == "" || file == route.Index {
		webapp = app.DoLazyUpdate(i, webapp, i.AppsCopier(consts.WebappType), i.Registries()).(*app.WebappManifest)
	}

	switch webapp.State() {
	case app.Installed:
		return c.Redirect(http.StatusFound, i.PageURL("/auth/authorize/app", url.Values{
			"slug": {slug},
		}))
	case app.Ready:
		return ServeAppFile(c, i, i.AppsFileServer(), webapp)
	default:
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Application is not ready")
	}
}

// handleIntent will allow iframes from another app if the current app is
// opened as an intent
func handleIntent(c echo.Context, i *instance.Instance, slug, intentID string) {
	intent := &intent.Intent{}
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
	middlewares.AppendCSPRule(c, "frame-ancestors", from)
}

// ServeAppFile will serve the requested file using the specified application
// manifest and appfs.FileServer context.
//
// It can be used to serve file application in another context than the VFS,
// for instance for tests or development purposes where we want to serve an
// application that is not installed on the user's instance. However this
// procedure should not be used for standard applications, use the Serve method
// for that.
func ServeAppFile(c echo.Context, i *instance.Instance, fs appfs.FileServer, webapp *app.WebappManifest) error {
	slug := webapp.Slug()
	route, file := webapp.FindRoute(path.Clean(c.Request().URL.Path))
	if route.NotFound() {
		return echo.NewHTTPError(http.StatusNotFound, "Page not found")
	}
	if file == "" {
		file = route.Index
	}

	session, isLoggedIn := middlewares.GetSession(c)
	filepath := path.Join("/", route.Folder, file)
	isRobotsTxt := filepath == "/robots.txt"

	if !route.Public && !isLoggedIn && !isRobotsTxt {
		if file != route.Index {
			return echo.NewHTTPError(http.StatusUnauthorized, "You must be authenticated")
		}
		reqURL := c.Request().URL
		subdomain := i.SubDomain(slug)
		subdomain.Path = reqURL.Path
		subdomain.RawQuery = reqURL.RawQuery
		subdomain.Fragment = reqURL.Fragment
		params := url.Values{
			"redirect": {subdomain.String()},
		}
		if jwt := c.QueryParam("jwt"); jwt != "" {
			params.Add("jwt", jwt)
		}
		return c.Redirect(http.StatusFound, i.PageURL("/auth/login", params))
	}

	version := webapp.Version()
	shasum := webapp.Checksum()

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

		err := fs.ServeFileContent(c.Response(), c.Request(), slug, version, shasum, filepath)
		if os.IsNotExist(err) {
			if isRobotsTxt {
				if f, ok := statikfs.Get("/robots.txt", i.ContextName); ok {
					_, err = io.Copy(c.Response(), f.Reader())
					return err
				}
				if f, ok := statikfs.Get("/robots.txt", ""); ok {
					_, err = io.Copy(c.Response(), f.Reader())
					return err
				}
			}
			return echo.NewHTTPError(http.StatusNotFound, "Asset not found")
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
	content, err := fs.Open(slug, version, shasum, filepath)
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
		i.Logger().WithField("nspace", "apps").Warnf("%s cannot be parsed as a template: %s", file, err)
		return fs.ServeFileContent(c.Response(), c.Request(), slug, version, shasum, filepath)
	}

	var token string
	if isLoggedIn {
		token = i.BuildAppToken(webapp.Slug(), session.ID())
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
		"Token":         token,
		"Domain":        i.ContextualDomain(),
		"ContextName":   i.ContextName,
		"Locale":        i.Locale,
		"AppSlug":       webapp.Slug(),
		"AppName":       webapp.NameLocalized(i.Locale),
		"AppEditor":     webapp.Editor,
		"AppNamePrefix": webapp.NamePrefix,
		"IconPath":      webapp.Icon,
		"CozyBar":       cozybar(i, isLoggedIn),
		"CozyClientJS":  cozyclientjs(i),
		"ThemeCSS":      middlewares.ThemeCSS(i),
		"Tracking":      tracking,
		"Favicon":       middlewares.Favicon(i),
	})
}

func tryAuthWithSessionCode(c echo.Context, i *instance.Instance, value, slug string) error {
	u := *(c.Request().URL)
	u.Scheme = i.Scheme()
	u.Host = c.Request().Host
	if code := session.FindCode(value, u.Host); code != nil {
		sess, err := session.Get(i, code.SessionID)
		if err == nil {
			cookie, err := sess.ToAppCookie(u.Host, slug)
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
		Name:   session.SessionCookieName,
		Value:  "",
		MaxAge: -1,
		Path:   "/",
		Domain: utils.StripPort(i.ContextualDomain()),
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

var clientTemplate *template.Template
var barTemplate *template.Template

// BuildTemplates ensure that cozy-client-js and the bar can be injected in templates
func BuildTemplates() {
	clientTemplate = template.Must(template.New("cozy-client-js").Funcs(middlewares.FuncsMap).Parse(`` +
		`<script src="{{asset .Domain "/js/cozy-client.min.js" .ContextName}}"></script>`,
	))

	barTemplate = template.Must(template.New("cozy-bar").Funcs(middlewares.FuncsMap).Parse(`
<link rel="stylesheet" type="text/css" href="{{asset .Domain "/fonts/fonts.css" .ContextName}}">
<link rel="stylesheet" type="text/css" href="{{asset .Domain "/css/cozy-bar.min.css" .ContextName}}">
{{if .LoggedIn}}
{{range .Warnings}}
<meta name="user-action-required" data-title="{{ .Title }}" data-code="{{ .Code }}" data-detail="{{ .Detail }}" data-links="{{ .Links.Self }}" />
{{end}}
{{end}}
<script src="{{asset .Domain "/js/cozy-bar.min.js" .ContextName}}"></script>`,
	))

}

func cozyclientjs(i *instance.Instance) template.HTML {
	buf := new(bytes.Buffer)
	err := clientTemplate.Execute(buf, echo.Map{
		"Domain":      i.ContextualDomain(),
		"ContextName": i.ContextName,
	})
	if err != nil {
		panic(err)
	}
	return template.HTML(buf.String())
}

func cozybar(i *instance.Instance, loggedIn bool) template.HTML {
	buf := new(bytes.Buffer)
	err := barTemplate.Execute(buf, echo.Map{
		"Domain":      i.ContextualDomain(),
		"Warnings":    i.Warnings(),
		"ContextName": i.ContextName,
		"LoggedIn":    loggedIn,
	})
	if err != nil {
		panic(err)
	}
	return template.HTML(buf.String())
}
