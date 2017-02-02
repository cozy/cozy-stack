//go:generate statik -src=../.assets -dest=.

package web

import (
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/web/apps"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/data"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/instances"
	"github.com/cozy/cozy-stack/web/jobs"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/cozy/cozy-stack/web/settings"
	_ "github.com/cozy/cozy-stack/web/statik" // Generated file with the packed assets
	"github.com/cozy/cozy-stack/web/status"
	"github.com/cozy/cozy-stack/web/version"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/rakyll/statik/fs"
)

var (
	hstsMaxAge = 365 * 24 * time.Hour // 1 year

	templatesList = []string{
		"authorize.html",
		"error.html",
		"login.html",
	}
)

type renderer struct {
	t *template.Template
	h http.Handler
}

func (r *renderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return r.t.ExecuteTemplate(w, name, data)
}

func newRenderer(assetsPath string) (*renderer, error) {
	// By default, use the assets packed in the binary
	if assetsPath != "" {
		list := make([]string, len(templatesList))
		for i, name := range templatesList {
			list[i] = path.Join(assetsPath, "templates", name)
		}
		t, err := template.ParseFiles(list...)
		if err != nil {
			return nil, err
		}
		h := http.FileServer(http.Dir(assetsPath))
		r := &renderer{t, h}
		return r, nil
	}

	statikFS, err := fs.New()
	if err != nil {
		return nil, err
	}

	var t, tmpl *template.Template
	for _, name := range templatesList {
		if t == nil {
			t = template.New(name)
			tmpl = t
		} else {
			tmpl = t.New(name)
		}
		f, err := statikFS.Open("/templates/" + name)
		if err != nil {
			return nil, err
		}
		b, err := ioutil.ReadAll(f)
		if err != nil {
			return nil, err
		}
		_, err = tmpl.Parse(string(b))
		if err != nil {
			return nil, err
		}
	}

	h := http.FileServer(statikFS)
	r := &renderer{t, h}
	return r, nil
}

// SetupAppsHandler adds all the necessary middlewares for the application
// handler.
func SetupAppsHandler(appsHandler echo.HandlerFunc) echo.HandlerFunc {
	secure := middlewares.Secure(&middlewares.SecureConfig{
		HSTSMaxAge:    hstsMaxAge,
		CSPDefaultSrc: []middlewares.CSPSource{middlewares.CSPSrcSelf, middlewares.CSPSrcParent},
		CSPFontSrc:    []middlewares.CSPSource{middlewares.CSPSrcSelf, middlewares.CSPSrcData, middlewares.CSPSrcParent},
		CSPImgSrc:     []middlewares.CSPSource{middlewares.CSPSrcSelf, middlewares.CSPSrcData, middlewares.CSPSrcParent},
		CSPFrameSrc:   []middlewares.CSPSource{middlewares.CSPSrcParent},
		XFrameOptions: middlewares.XFrameDeny,
	})

	return middlewares.Compose(appsHandler, secure, middlewares.LoadSession)
}

// SetupAssets add assets routing and handling to the given router. It also
// adds a Renderer to render templates.
func SetupAssets(router *echo.Echo, assetsPath string) error {
	r, err := newRenderer(assetsPath)
	if err != nil {
		return err
	}

	router.Renderer = r
	router.GET("/assets/*", echo.WrapHandler(http.StripPrefix("/assets/", r.h)))
	router.GET("/favicon.ico", echo.WrapHandler(r.h))
	router.GET("/robots.txt", echo.WrapHandler(r.h))
	return nil
}

// SetupRoutes sets the routing for HTTP endpoints
func SetupRoutes(router *echo.Echo) error {
	secure := middlewares.Secure(&middlewares.SecureConfig{
		HSTSMaxAge:    hstsMaxAge,
		CSPDefaultSrc: []middlewares.CSPSource{middlewares.CSPSrcSelf},
		XFrameOptions: middlewares.XFrameDeny,
	})

	router.Use(secure, middlewares.CORS)

	mws := []echo.MiddlewareFunc{
		middlewares.NeedInstance,
		middlewares.LoadSession,
	}
	auth.Routes(router.Group("/auth", mws...))
	apps.Routes(router.Group("/apps", mws...))
	data.Routes(router.Group("/data", mws...))
	files.Routes(router.Group("/files", mws...))
	jobs.Routes(router.Group("/jobs", mws...))
	permissions.Routes(router.Group("/permissions", mws...))
	settings.Routes(router.Group("/settings", mws...))
	status.Routes(router.Group("/status"))
	version.Routes(router.Group("/version"))

	setupRecover(router)

	router.HTTPErrorHandler = errors.ErrorHandler
	return nil
}

// SetupAdminRoutes sets the routing for the administration HTTP endpoints
func SetupAdminRoutes(router *echo.Echo) error {
	if !config.IsDevRelease() {
		router.Use(middlewares.BasicAuth(config.AdminSecretFileName))
	}

	instances.Routes(router.Group("/instances"))

	setupRecover(router)

	router.HTTPErrorHandler = errors.ErrorHandler
	return nil
}

// CreateSubdomainProxy returns a new web server that will handle that apps
// proxy routing if the host of the request match an application, and route to
// the given router otherwise.
func CreateSubdomainProxy(router *echo.Echo, serveApps echo.HandlerFunc) (*echo.Echo, error) {
	if err := SetupAssets(router, config.GetConfig().Assets); err != nil {
		return nil, err
	}

	if err := SetupRoutes(router); err != nil {
		return nil, err
	}

	serveApps = SetupAppsHandler(serveApps)

	main := echo.New()
	main.Any("/*", func(c echo.Context) error {
		// TODO(optim): minimize the number of instance requests
		if parent, slug := middlewares.SplitHost(c.Request().Host); slug != "" {
			if i, err := instance.Get(parent); err == nil {
				c.Set("instance", i)
				c.Set("slug", slug)
				return serveApps(c)
			}
		}

		router.ServeHTTP(c.Response(), c.Request())
		return nil
	})

	main.HTTPErrorHandler = errors.ErrorHandler
	return main, nil
}

// setupRecover sets a recovering strategy of panics happening in handlers
func setupRecover(router *echo.Echo) {
	recoverMiddleware := middleware.RecoverWithConfig(middleware.RecoverConfig{
		StackSize:         1 << 10, // 1 KB
		DisableStackAll:   !config.IsDevRelease(),
		DisablePrintStack: !config.IsDevRelease(),
	})
	router.Use(recoverMiddleware)
}
