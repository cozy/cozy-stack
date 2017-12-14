//go:generate statik -src=../assets -dest=. -externals=../assets/.externals

package web

import (
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/i18n"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/metrics"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/web/apps"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/data"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/instances"
	"github.com/cozy/cozy-stack/web/intents"
	"github.com/cozy/cozy-stack/web/jobs"
	"github.com/cozy/cozy-stack/web/konnectorsauth"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/notifications"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/cozy/cozy-stack/web/realtime"
	"github.com/cozy/cozy-stack/web/registry"
	"github.com/cozy/cozy-stack/web/remote"
	"github.com/cozy/cozy-stack/web/settings"
	"github.com/cozy/cozy-stack/web/sharings"
	_ "github.com/cozy/cozy-stack/web/statik" // Generated file with the packed assets
	"github.com/cozy/cozy-stack/web/status"
	"github.com/cozy/cozy-stack/web/version"

	statikFS "github.com/cozy/statik/fs"
	"github.com/labstack/echo"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	hstsMaxAge = 365 * 24 * time.Hour // 1 year

	templatesList = []string{
		"authorize.html",
		"authorize_app.html",
		"authorize_sharing.html",
		"error.html",
		"login.html",
		"need_onboarding.html",
		"passphrase_reset.html",
		"passphrase_renew.html",
		"sharing_discovery.html",
	}

	privateAssets = []string{
		"/templates/",
		"/locales/",
		"/externals",
	}
)

type renderer struct {
	http.Handler
	t *template.Template
}

func (r *renderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	var funcMap template.FuncMap
	i, ok := middlewares.GetInstanceSafe(c)
	if ok {
		funcMap = template.FuncMap{"t": i.Translate}
	} else {
		// TODO: improve language detection with a package like
		// "golang.org/x/text/language"
		lang := i18n.DefaultLocale
		acceptLanguage := c.Request().Header.Get("Accept-Language")
		acceptLanguageSplit := strings.SplitN(acceptLanguage, ";", 2)
		if len(acceptLanguage) >= 1 {
			langs := utils.SplitTrimString(acceptLanguageSplit[0], ",")
			for _, l := range langs {
				if utils.IsInArray(l, i18n.SupportedLocales) {
					lang = l
					break
				}
			}
		}
		funcMap = template.FuncMap{"t": i18n.Translator(lang)}
	}
	t, err := r.t.Clone()
	if err != nil {
		return err
	}
	return t.Funcs(funcMap).ExecuteTemplate(w, name, data)
}

func newRenderer(assetsPath string) (*renderer, error) {
	funcsMap := template.FuncMap{
		"t":     fmt.Sprintf,
		"Split": strings.Split,
	}

	// By default, use the assets packed in the binary
	if assetsPath != "" {
		list := make([]string, len(templatesList))
		for i, name := range templatesList {
			list[i] = path.Join(assetsPath, "templates", name)
		}
		var err error
		t := template.New("stub").Funcs(funcsMap)
		if t, err = t.ParseFiles(list...); err != nil {
			return nil, fmt.Errorf("Can't load the assets from %q: %s", assetsPath, err)
		}
		h := http.FileServer(http.Dir(assetsPath))
		r := &renderer{t: t, Handler: http.StripPrefix("/assets", h)}
		return r, nil
	}

	var t, tmpl *template.Template
	for _, name := range templatesList {
		if t == nil {
			t = template.New(name)
			tmpl = t
		} else {
			tmpl = t.New(name)
		}
		tmpl = tmpl.Funcs(funcsMap)
		f, err := statikFS.Open("/templates/" + name)
		if err != nil {
			return nil, fmt.Errorf("Can't load asset %q: %s", name, err)
		}
		b, err := ioutil.ReadAll(f)
		if err != nil {
			return nil, err
		}
		if _, err = tmpl.Parse(string(b)); err != nil {
			return nil, err
		}
	}

	r := &renderer{t: t, Handler: statikFS.Handler("/assets", privateAssets...)}
	return r, nil
}

// SetupAppsHandler adds all the necessary middlewares for the application
// handler.
func SetupAppsHandler(appsHandler echo.HandlerFunc) echo.HandlerFunc {
	mws := []echo.MiddlewareFunc{
		middlewares.LoadAppSession,
	}
	if !config.GetConfig().DisableCSP {
		secure := middlewares.Secure(&middlewares.SecureConfig{
			HSTSMaxAge:    hstsMaxAge,
			CSPDefaultSrc: []middlewares.CSPSource{middlewares.CSPSrcSelf, middlewares.CSPSrcParent, middlewares.CSPSrcWS},
			CSPStyleSrc:   []middlewares.CSPSource{middlewares.CSPSrcSelf, middlewares.CSPSrcParent, middlewares.CSPUnsafeInline},
			CSPFontSrc:    []middlewares.CSPSource{middlewares.CSPSrcSelf, middlewares.CSPSrcData, middlewares.CSPSrcParent},
			CSPImgSrc:     []middlewares.CSPSource{middlewares.CSPSrcSelf, middlewares.CSPSrcData, middlewares.CSPSrcBlob, middlewares.CSPSrcParent, middlewares.CSPSrcWhitelist},
			CSPFrameSrc:   []middlewares.CSPSource{middlewares.CSPSrcSiblings},
			XFrameOptions: middlewares.XFrameSameOrigin,
		})
		mws = append([]echo.MiddlewareFunc{secure}, mws...)
	}

	return middlewares.Compose(appsHandler, mws...)
}

// SetupAssets add assets routing and handling to the given router. It also
// adds a Renderer to render templates.
func SetupAssets(router *echo.Echo, assetsPath string) error {
	r, err := newRenderer(assetsPath)
	if err != nil {
		return err
	}

	router.Renderer = r
	router.GET("/assets/*", echo.WrapHandler(r))
	router.GET("/favicon.ico", echo.WrapHandler(r))
	router.GET("/robots.txt", echo.WrapHandler(r))
	return nil
}

// SetupRoutes sets the routing for HTTP endpoints
func SetupRoutes(router *echo.Echo) error {
	router.Use(timersMiddleware)

	if !config.GetConfig().DisableCSP {
		secure := middlewares.Secure(&middlewares.SecureConfig{
			HSTSMaxAge:    hstsMaxAge,
			CSPDefaultSrc: []middlewares.CSPSource{middlewares.CSPSrcSelf},
			// Display logos of OAuth clients on the authorize page
			CSPImgSrc:     []middlewares.CSPSource{middlewares.CSPSrcAny},
			XFrameOptions: middlewares.XFrameDeny,
		})
		router.Use(secure)
	}

	router.Use(middlewares.CORS)

	mws := []echo.MiddlewareFunc{
		middlewares.NeedInstance,
		middlewares.LoadSession,
	}
	router.GET("/", auth.Home, mws...)
	// accounts routes does not all use middlewares
	konnectorsauth.Routes(router.Group("/accounts"))
	auth.Routes(router.Group("/auth", mws...))
	apps.WebappsRoutes(router.Group("/apps", mws...))
	apps.KonnectorRoutes(router.Group("/konnectors", mws...))
	registry.Routes(router.Group("/registry", mws...))
	data.Routes(router.Group("/data", mws...))
	files.Routes(router.Group("/files", mws...))
	intents.Routes(router.Group("/intents", mws...))
	jobs.Routes(router.Group("/jobs", mws...))
	notifications.Routes(router.Group("/notifications", mws...))
	permissions.Routes(router.Group("/permissions", mws...))
	realtime.Routes(router.Group("/realtime", mws...))
	remote.Routes(router.Group("/remote", mws...))
	settings.Routes(router.Group("/settings", mws...))
	sharings.Routes(router.Group("/sharings", mws...))
	status.Routes(router.Group("/status"))
	version.Routes(router.Group("/version"))

	setupRecover(router)

	router.HTTPErrorHandler = errors.ErrorHandler
	return nil
}

func timersMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
			status := strconv.Itoa(c.Response().Status)
			metrics.HTTPTotalDurations.
				WithLabelValues(c.Request().Method, status).
				Observe(v)
		}))
		defer timer.ObserveDuration()
		return next(c)
	}
}

// SetupAdminRoutes sets the routing for the administration HTTP endpoints
func SetupAdminRoutes(router *echo.Echo) error {
	var mws []echo.MiddlewareFunc
	if !config.IsDevRelease() {
		mws = append(mws, middlewares.BasicAuth(config.AdminSecretFileName))
	}

	// XXX Do not set basic authentication on the /metrics routes.
	metrics.Routes(router.Group("/metrics"))

	instances.Routes(router.Group("/instances", mws...))
	version.Routes(router.Group("/version", mws...))

	setupRecover(router)

	router.HTTPErrorHandler = errors.ErrorHandler
	return nil
}

// CreateSubdomainProxy returns a new web server that will handle that apps
// proxy routing if the host of the request match an application, and route to
// the given router otherwise.
func CreateSubdomainProxy(router *echo.Echo, appsHandler echo.HandlerFunc) (*echo.Echo, error) {
	if err := SetupAssets(router, config.GetConfig().Assets); err != nil {
		return nil, err
	}

	if err := SetupRoutes(router); err != nil {
		return nil, err
	}

	appsHandler = SetupAppsHandler(appsHandler)

	main := echo.New()
	main.HideBanner = true
	main.Renderer = router.Renderer
	main.Any("/*", func(c echo.Context) error {
		// TODO(optim): minimize the number of instance requests
		if parent, slug, _ := middlewares.SplitHost(c.Request().Host); slug != "" {
			if i, err := instance.Get(parent); err == nil {
				c.Set("instance", i)
				c.Set("slug", slug)
				return appsHandler(c)
			}
		}

		router.ServeHTTP(c.Response(), c.Request())
		return nil
	})

	main.HTTPErrorHandler = errors.HTMLErrorHandler
	return main, nil
}

// setupRecover sets a recovering strategy of panics happening in handlers
func setupRecover(router *echo.Echo) {
	if !config.IsDevRelease() {
		recoverMiddleware := middlewares.RecoverWithConfig(middlewares.RecoverConfig{
			StackSize: 10 << 10, // 10KB
		})
		router.Use(recoverMiddleware)
	}
}
