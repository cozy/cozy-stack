// Package web Cozy Stack API.
//
// Cozy is a personal platform as a service with a focus on data.
package web

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/pkg/assets"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/i18n"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/web/apps"
	"github.com/sirupsen/logrus"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
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
		for _, locale := range consts.SupportedLocales {
			pofile := path.Join(assetsPath, "locales", locale+".po")
			po, err := os.ReadFile(pofile)
			if err != nil {
				return fmt.Errorf("Can't load the po file for %s", locale)
			}
			i18n.LoadLocale(locale, "", po)
		}
		return nil
	}

	for _, locale := range consts.SupportedLocales {
		f, err := assets.Open("/locales/"+locale+".po", config.DefaultInstanceContext)
		if err != nil {
			return fmt.Errorf("Can't load the po file for %s", locale)
		}
		po, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		i18n.LoadLocale(locale, "", po)
	}
	return nil
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
			logger.WithNamespace("dev").Warnf("Directory %s does not exist", dir)
		} else {
			if err = checkExists(path.Join(dir, app.WebappManifestName)); err != nil {
				logger.WithNamespace("dev").Warnf("The app manifest is missing: %s", err)
			}
			if err = checkExists(path.Join(dir, "index.html")); err != nil {
				logger.WithNamespace("dev").Warnf("The index.html is missing: %s", err)
			}
		}
	}

	app.SetupAppsDir(appsdir)
	return ListenAndServe()
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

// ListenAndServe creates and setups all the necessary http endpoints and start
// them.
func ListenAndServe() (*Servers, error) {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	major, err := CreateSubdomainProxy(e, apps.Serve)
	if err != nil {
		return nil, err
	}
	if err = LoadSupportedLocales(); err != nil {
		return nil, err
	}

	if build.IsDevRelease() {
		timeFormat := "time_rfc3339"
		if logrus.GetLevel() == logrus.DebugLevel {
			timeFormat = "time_rfc3339_nano"
		}
		major.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
			Format: "time=${" + timeFormat + "}\tstatus=${status}\tmethod=${method}\thost=${host}\turi=${uri}\tbytes_out=${bytes_out}\n",
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

	e.start(e.major, "major", config.ServerAddr())
	e.start(e.admin, "admin", config.AdminServerAddr())
}

func (e *Servers) start(s *echo.Echo, name string, addr string) {
	hosts := []string{}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		panic(err)
	}

	switch host {
	case "localhost":
		hosts = append(hosts, "127.0.0.1", "::1")
	default:
		hosts = append(hosts, host)
	}

	for _, h := range hosts {
		addr := net.JoinHostPort(h, port)

		fmt.Printf("http server %s started on %q\n", name, addr)
		go func(addr string) {
			e.errs <- s.StartServer(&http.Server{
				Addr:              addr,
				ReadHeaderTimeout: ReadHeaderTimeout,
			})
		}(addr)
	}
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
