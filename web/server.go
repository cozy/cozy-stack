// Package web Cozy Stack API.
//
// Cozy is a personal platform as a service with a focus on data.
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/utils"
	webapps "github.com/cozy/cozy-stack/web/apps"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/spf13/afero"
)

// ListenAndServe creates and setups all the necessary http endpoints and start
// them.
func ListenAndServe() error {
	return listenAndServe(webapps.Serve)
}

// ListenAndServeWithAppDir creates and setup all the necessary http endpoints
// and serve the specified application on a app subdomain.
//
// In order to serve the application, the specified directory should provide
// a manifest.webapp file that will be used to parameterize the application
// permissions.
func ListenAndServeWithAppDir(dir string) error {
	dir = utils.AbsPath(dir)
	exists, err := utils.DirExists(dir)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("Directory %s does not exist", dir)
	}
	if err = checkExists(path.Join(dir, apps.ManifestFilename)); err != nil {
		return err
	}
	if err = checkExists(path.Join(dir, "index.html")); err != nil {
		return err
	}
	return listenAndServe(func(c echo.Context) error {
		slug := c.Get("slug").(string)
		if slug != "app" {
			return webapps.Serve(c)
		}
		method := c.Request().Method
		if method != "GET" && method != "HEAD" {
			return echo.NewHTTPError(http.StatusMethodNotAllowed, "Method %s not allowed", method)
		}
		fs := afero.NewBasePathFs(afero.NewOsFs(), dir)
		manFile, err := fs.Open(apps.ManifestFilename)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("Could not find the %s file in your application directory %s",
					apps.ManifestFilename, dir)
			}
			return err
		}
		app := &apps.Manifest{}
		if err = json.NewDecoder(manFile).Decode(&app); err != nil {
			return fmt.Errorf("Could not parse the %s file: %s",
				apps.ManifestFilename, err.Error())
		}
		app.CreateDefaultRoute()
		app.Slug = slug
		i := middlewares.GetInstance(c)
		f := webapps.NewAferoServer(fs, func(_, folder, file string) string {
			return path.Join(folder, file)
		})
		// Save permissions in couchdb before loading an index page
		if _, file := app.FindRoute(path.Clean(c.Request().URL.Path)); file == "" {
			if app.Permissions != nil {
				if err := permissions.Force(i, app.Slug, *app.Permissions); err != nil {
					return err
				}
			}
		}
		return webapps.ServeAppFile(c, i, f, app)
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

func listenAndServe(appsHandler echo.HandlerFunc) error {
	main, err := CreateSubdomainProxy(echo.New(), appsHandler)
	if err != nil {
		return err
	}

	admin := echo.New()
	if err = SetupAdminRoutes(admin); err != nil {
		return err
	}

	if config.IsDevRelease() {
		fmt.Println(`                           !! DEVELOPMENT RELEASE !!
You are running a development release which may deactivate some very important
security features. Please do not use this binary as your production server.
`)
		main.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
			Format: "time=${time_rfc3339}\tstatus=${status}\tmethod=${method}\thost=${host}\turi=${uri}\tbytes_out=${bytes_out}\n",
		}))
	}

	errs := make(chan error)
	go func() { errs <- admin.Start(config.AdminServerAddr()) }()
	go func() { errs <- main.Start(config.ServerAddr()) }()
	return <-errs
}
