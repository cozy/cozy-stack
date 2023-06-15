// Package web Cozy Stack API.
//
// Cozy is a personal platform as a service with a focus on data.
package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
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

var (
	ErrMissingArgument = errors.New("the argument is missing")
)

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

	servers := NewServers()
	err = servers.Start(major, "major", config.ServerAddr())
	if err != nil {
		return nil, fmt.Errorf("failed to start major server: %w", err)
	}

	err = servers.Start(admin, "admin", config.AdminServerAddr())
	if err != nil {
		return nil, fmt.Errorf("failed to start admin server: %w", err)
	}

	return servers, nil
}

// Servers allow to start several [echo.Echo] servers and stop them together.
//
//	It also take care of several other task:
//	- It sanitize the hosts format
//	- It exposes the handlers on several addresses if needed
//	- It forces the IPv4/IPv6 dual stack mode for `localhost` by
//	  remplacing this entry by `["127.0.0.1", "::1]`
type Servers struct {
	serversByName   map[string]*http.Server
	listenersByName map[string]net.Listener
	errs            chan error
}

func NewServers() *Servers {
	return &Servers{
		serversByName:   map[string]*http.Server{},
		listenersByName: map[string]net.Listener{},
		errs:            make(chan error),
	}
}

// Start the server 'e' to the given addrs.
//
// The 'addrs' arguments must be in the format `"host:port"`. If the host
// is not a valid IPv4/IPv6/hostname or if the port not present an error is
// returned.
func (s *Servers) Start(handler http.Handler, name string, addr string) error {
	addrs := []string{}

	if len(addr) == 0 {
		return fmt.Errorf("args: %w", ErrMissingArgument)
	}

	if len(name) == 0 {
		return fmt.Errorf("name: %w", ErrMissingArgument)
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}

	fmt.Printf("http server %s started on %q\n", name, addr)
	switch host {
	case "localhost":
		addrs = append(addrs, net.JoinHostPort("127.0.0.1", port))
		addrs = append(addrs, net.JoinHostPort("::1", port))
	default:
		addrs = append(addrs, net.JoinHostPort(host, port))
	}

	for _, addr := range addrs {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}

		writer := logger.WithNamespace("stack").Writer()
		logger := log.New(writer, "", 0)
		server := &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: ReadHeaderTimeout,
			ErrorLog:          logger,
		}

		s.serversByName[name] = server
		s.listenersByName[name] = l

		go func(server *http.Server) {
			s.errs <- server.Serve(l)
		}(server)
	}

	return nil
}

// GetAddr return the address where the given server listen to.
//
// This endpoint is mostly used when we use dynamic port attribution
// like when we don't specify a port
func (s *Servers) GetAddr(name string) net.Addr {
	l, ok := s.listenersByName[name]
	if !ok {
		return nil
	}

	return l.Addr()
}

// Wait for servers to stop or fall in error.
func (s *Servers) Wait() <-chan error {
	return s.errs
}

// Shutdown gracefully stops the servers.
func (s *Servers) Shutdown(ctx context.Context) error {
	shutdowners := []utils.Shutdowner{}

	for _, server := range s.serversByName {
		shutdowners = append(shutdowners, server)
	}

	g := utils.NewGroupShutdown(shutdowners...)

	fmt.Print("  shutting down servers...")
	if err := g.Shutdown(ctx); err != nil {
		fmt.Println("failed: ", err.Error())
		return err
	}

	fmt.Println("ok.")

	return nil
}
