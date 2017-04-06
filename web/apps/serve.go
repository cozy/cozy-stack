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
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/intents"
	"github.com/cozy/cozy-stack/pkg/sessions"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
	"github.com/spf13/afero"
)

// Serve is an handler for serving files from the VFS for a client-side app
func Serve(c echo.Context) error {
	method := c.Request().Method
	if method != "GET" && method != "HEAD" {
		return echo.NewHTTPError(http.StatusMethodNotAllowed, "Method %s not allowed", method)
	}
	i := middlewares.GetInstance(c)
	if config.GetConfig().Subdomains == config.FlatSubdomains {
		if code := c.QueryParam("code"); code != "" {
			return tryAuthWithSessionCode(c, i, code)
		}
	}
	slug := c.Get("slug").(string)
	app, err := apps.GetWebappBySlug(i, slug)
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			return echo.NewHTTPError(http.StatusNotFound, "Application not found")
		}
		return err
	}
	if app.State() != apps.Ready {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Application is not ready")
	}
	return ServeAppFile(c, i, NewServer(i.AppsFS(apps.Webapp), nil), app)
}

func onboarding(c echo.Context) bool {
	i := middlewares.GetInstance(c)
	if len(i.RegisterToken) == 0 {
		return false
	}
	return c.QueryParam("registerToken") != ""
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
// manifest and AppFileServer context.
//
// It can be used to serve file application in another context than the VFS,
// for instance for tests or development puposes where we want to serve an
// application that is not installed on the user's instance. However this
// procedure should not be used for standard applications, use the Serve method
// for that.
func ServeAppFile(c echo.Context, i *instance.Instance, fs AppFileServer, app *apps.WebappManifest) error {
	slug := app.Slug()
	route, file := app.FindRoute(path.Clean(c.Request().URL.Path))
	if route.NotFound() {
		return echo.NewHTTPError(http.StatusNotFound, "Page not found")
	}
	needAuth := !route.Public
	if slug == consts.OnboardingSlug && file == "" && !onboarding(c) {
		needAuth = true
	}
	if needAuth && !middlewares.IsLoggedIn(c) {
		if file != "" {
			return echo.NewHTTPError(http.StatusUnauthorized, "You must be authenticated")
		}
		subdomain := i.SubDomain(slug)
		subdomain.Path = c.Request().URL.String()
		redirect := url.Values{
			"redirect": {subdomain.String()},
		}
		return c.Redirect(http.StatusFound, i.PageURL("/auth/login", redirect))
	}
	if file == "" {
		file = route.Index
	}
	infos, err := fs.Stat(slug, route.Folder, file)
	if os.IsNotExist(err) {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err)
	}
	modtime := infos.ModTime()
	if file != route.Index {
		return fs.ServeFileContent(c.Response(), c.Request(), modtime, slug, route.Folder, file)
	}
	if intentID := c.QueryParam("intent"); intentID != "" {
		handleIntent(c, i, slug, intentID)
	}
	// For index file, we inject the locale, the stack domain, and a token if the
	// user is connected
	content, err := fs.Open(slug, route.Folder, file)
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
		log.Warnf("[apps] %s cannot be parsed as a template: %s", file, err)
		return fs.ServeFileContent(c.Response(), c.Request(), modtime, slug, route.Folder, file)
	}
	token := "" // #nosec
	if middlewares.IsLoggedIn(c) {
		token = i.BuildAppToken(app)
	}
	res := c.Response()
	res.Header().Set("Content-Type", "text/html; charset=utf-8")
	res.WriteHeader(http.StatusOK)
	return tmpl.Execute(res, echo.Map{
		"Token":        token,
		"Domain":       i.Domain,
		"Locale":       i.Locale,
		"AppName":      app.Name,
		"IconPath":     app.Icon,
		"CozyBar":      cozybar(i),
		"CozyClientJS": cozyclientjs(i),
	})
}

// AppFileServer interface defines a way to access and serve the application's
// data files.
type AppFileServer interface {
	Stat(slug, folder, file string) (os.FileInfo, error)
	Open(slug, folder, file string) (vfs.File, error)
	ServeFileContent(w http.ResponseWriter, req *http.Request, modtime time.Time, slug, folder, file string) error
}

// NewServer returns a simple wrapper of the afero.Fs interface that
// provides the AppFileServer interface.
//
// You can provide a makePath method to define how the file name should be
// created from the application's slug, folder and file name. If not provided,
// the standard VFS concatenation (starting with vfs.WebappsDirName) is used.
func NewServer(fs afero.Fs, makePath func(slug, folder, file string) string) *Server {
	if makePath == nil {
		makePath = defaultMakePath
	}
	return &Server{
		mkPath: makePath,
		fs:     fs,
	}
}

// Server is a simple wrapper of a afero.Fs that provides the
// AppFileServer interface.
type Server struct {
	mkPath func(slug, folder, file string) string
	fs     afero.Fs
}

// Stat returns the underlying afero.Fs Stat.
func (s *Server) Stat(slug, folder, file string) (os.FileInfo, error) {
	return s.fs.Stat(s.mkPath(slug, folder, file))
}

// Open returns the underlying afero.Fs Open.
func (s *Server) Open(slug, folder, file string) (vfs.File, error) {
	return s.fs.Open(s.mkPath(slug, folder, file))
}

func defaultMakePath(slug, folder, file string) string {
	return path.Join("/", slug, folder, file)
}

// ServeFileContent uses the standard http.ServeContent method to serve the
// application file data.
func (s *Server) ServeFileContent(w http.ResponseWriter, req *http.Request, modtime time.Time, slug, folder, file string) error {
	filepath := s.mkPath(slug, folder, file)
	r, err := s.fs.Open(filepath)
	if err != nil {
		return err
	}
	defer r.Close()
	http.ServeContent(w, req, filepath, modtime, r)
	return nil
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
