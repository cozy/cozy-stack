//go:generate statik -src=../../assets -dest=..

package routing

import (
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"time"

	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/web/apps"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/data"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/instances"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/settings"
	_ "github.com/cozy/cozy-stack/web/statik" // Generated file with the packed assets
	"github.com/cozy/cozy-stack/web/status"
	"github.com/cozy/cozy-stack/web/version"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/rakyll/statik/fs"
)

var templatesList = []string{
	"authorize.html",
	"error.html",
	"login.html",
}

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
	cors := middleware.CORSWithConfig(middleware.CORSConfig{
		MaxAge: int(12 * time.Hour / time.Second),
	})

	auth.Routes(router.Group("", middlewares.NeedInstance))
	apps.Routes(router.Group("/apps", cors, middlewares.NeedInstance))
	data.Routes(router.Group("/data", cors, middlewares.NeedInstance))
	files.Routes(router.Group("/files", cors, middlewares.NeedInstance))
	settings.Routes(router.Group("/settings", cors, middlewares.NeedInstance))
	status.Routes(router.Group("/status", cors))
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

// setupRecover sets a recovering strategy of panics happening in handlers
func setupRecover(router *echo.Echo) {
	recoverMiddleware := middleware.RecoverWithConfig(middleware.RecoverConfig{
		StackSize:         1 << 10, // 1 KB
		DisableStackAll:   !config.IsDevRelease(),
		DisablePrintStack: !config.IsDevRelease(),
	})
	router.Use(recoverMiddleware)
}
