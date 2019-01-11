// Package web Cozy Stack API.
//
// Cozy is a personal platform as a service with a focus on data.
package web

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/i18n"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/utils"
	webapps "github.com/cozy/cozy-stack/web/apps"
	"github.com/cozy/cozy-stack/web/middlewares"

	"github.com/cozy/echo"
	"github.com/cozy/echo/middleware"

	"github.com/cozy/afero"
	statikFS "github.com/cozy/cozy-stack/pkg/statik/fs"
)

// ReadHeaderTimeout is the amount of time allowed to read request headers for
// all servers. This is activated for all handlers from all http servers
// created by the stack.
var ReadHeaderTimeout = 15 * time.Second

// LoadSupportedLocales reads the po files packed in go or from the assets directory
// and loads them for translations
func LoadSupportedLocales() error {
	// By default, use the po files packed in the binary
	// but use assets from the disk is assets option is filled in config
	assetsPath := config.GetConfig().Assets
	if assetsPath != "" {
		for _, locale := range i18n.SupportedLocales {
			pofile := path.Join(assetsPath, "locales", locale+".po")
			po, err := ioutil.ReadFile(pofile)
			if err != nil {
				return fmt.Errorf("Can't load the po file for %s", locale)
			}
			i18n.LoadLocale(locale, po)
		}
		return nil
	}

	for _, locale := range i18n.SupportedLocales {
		f, err := statikFS.Open("/locales/" + locale + ".po")
		if err != nil {
			return fmt.Errorf("Can't load the po file for %s", locale)
		}
		po, err := ioutil.ReadAll(f)
		if err != nil {
			return err
		}
		i18n.LoadLocale(locale, po)
	}
	return nil
}

// ListenAndServe creates and setups all the necessary http endpoints and start
// them.
func ListenAndServe() (*Servers, error) {
	return listenAndServe(webapps.Serve)
}

// ListenAndServeWithAppDir creates and setup all the necessary http endpoints
// and serve the specified application on a app subdomain.
//
// In order to serve the application, the specified directory should provide
// a manifest.webapp file that will be used to parameterize the application
// permissions.
func ListenAndServeWithAppDir(appsdir map[string]string) (*Servers, error) {
	for slug, dir := range appsdir {
		dir = utils.AbsPath(dir)
		appsdir[slug] = dir
		exists, err := utils.DirExists(dir)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("Directory %s does not exist", dir)
		}
		if err = checkExists(path.Join(dir, apps.WebappManifestName)); err != nil {
			return nil, err
		}
		if err = checkExists(path.Join(dir, "index.html")); err != nil {
			return nil, err
		}
	}
	return listenAndServe(func(c echo.Context) error {
		slug := c.Get("slug").(string)
		dir, ok := appsdir[slug]
		if !ok {
			return webapps.Serve(c)
		}
		method := c.Request().Method
		if method != "GET" && method != "HEAD" {
			return echo.NewHTTPError(http.StatusMethodNotAllowed, "Method not allowed")
		}
		fs := afero.NewBasePathFs(afero.NewOsFs(), dir)
		manFile, err := fs.Open(apps.WebappManifestName)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("Could not find the %s file in your application directory %s",
					apps.WebappManifestName, dir)
			}
			return err
		}
		var app apps.Manifest = &apps.WebappManifest{}
		app, err = app.ReadManifest(manFile, slug, "file://localhost"+dir)
		if err != nil {
			return fmt.Errorf("Could not parse the %s file: %s",
				apps.WebappManifestName, err.Error())
		}
		webapp := app.(*apps.WebappManifest)
		i := middlewares.GetInstance(c)
		f := apps.NewAferoFileServer(fs, func(_, _, _, file string) string {
			return path.Join("/", file)
		})
		// Save permissions in couchdb before loading an index page
		if _, file := webapp.FindRoute(path.Clean(c.Request().URL.Path)); file == "" {
			if webapp.Permissions() != nil {
				if err := permissions.ForceWebapp(i, webapp.Slug(), webapp.Permissions()); err != nil {
					return err
				}
			}
		}
		return webapps.ServeAppFile(c, i, f, webapp)
	})
}

func checkExists(filepath string) error {
	exists, err := utils.FileExists(filepath)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("Directory %s should contain a %s file",
			path.Dir(filepath), path.Base(filepath))
	}
	return nil
}

func listenAndServe(appsHandler echo.HandlerFunc) (*Servers, error) {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	major, err := CreateSubdomainProxy(e, appsHandler)
	if err != nil {
		return nil, err
	}
	if err = LoadSupportedLocales(); err != nil {
		return nil, err
	}

	if config.IsDevRelease() {
		major.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
			Format: "time=${time_rfc3339}\tstatus=${status}\tmethod=${method}\thost=${host}\turi=${uri}\tbytes_out=${bytes_out}\n",
		}))
	}

	admin := echo.New()
	admin.HideBanner = true
	admin.HidePort = true

	if err = SetupAdminRoutes(admin); err != nil {
		return nil, err
	}

	return &Servers{
		major: major,
		admin: admin,
	}, nil
}

// Servers contains the started HTTP servers and implement the Shutdowner
// interface.
type Servers struct {
	major *echo.Echo
	admin *echo.Echo
	errs  chan error
}

// Start starts the servers.
func (e *Servers) Start() {
	e.errs = make(chan error)

	go e.start(e.major, "major", &http.Server{
		Addr:              config.ServerAddr(),
		ReadHeaderTimeout: ReadHeaderTimeout,
	})

	go e.start(e.admin, "admin", &http.Server{
		Addr:              config.AdminServerAddr(),
		ReadHeaderTimeout: ReadHeaderTimeout,
	})
}

func (e *Servers) start(s *echo.Echo, name string, server *http.Server) {
	fmt.Printf("  http server %s started on %q\n", name, server.Addr)
	e.errs <- s.StartServer(server)
}

// Wait for servers to stop or fall in error.
func (e *Servers) Wait() <-chan error {
	return e.errs
}

// Shutdown gracefully stops the servers.
func (e *Servers) Shutdown(ctx context.Context) error {
	g := utils.NewGroupShutdown(e.admin, e.major)
	fmt.Print("  shutting down servers...")
	if err := g.Shutdown(ctx); err != nil {
		fmt.Println("failed: ", err.Error())
		return err
	}
	fmt.Println("ok.")
	return nil
}
