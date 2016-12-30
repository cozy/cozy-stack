package cmd

import (
	"fmt"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/web"
	"github.com/cozy/cozy-stack/web/apps"
	"github.com/cozy/cozy-stack/web/routing"
	"github.com/labstack/echo"
	"github.com/spf13/cobra"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Starts the stack and listens for HTTP calls",
	Long: `Starts the stack and listens for HTTP calls
It will accept HTTP requests on localhost:8080 by default.
Use the --port and --host flags to change the listening option.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		router := echo.New()

		if err := routing.SetupAssets(router, config.GetConfig().Assets); err != nil {
			return err
		}

		if err := routing.SetupRoutes(router); err != nil {
			return err
		}

		main, err := web.Create(router, apps.Serve)
		if err != nil {
			return err
		}

		admin := echo.New()
		if err := routing.SetupAdminRoutes(admin); err != nil {
			return err
		}

		if config.IsDevRelease() {
			fmt.Println(`                       !! DEVELOPMENT RELEASE !!
You are running a development release which may deactivate some very important
security features. Please do not use this binary as your production server.
`)
		}

		errs := make(chan error)

		go func() {
			errs <- admin.Start(config.AdminServerAddr())
		}()

		go func() {
			errs <- main.Start(config.ServerAddr())
		}()

		return <-errs
	},
}

func init() {
	RootCmd.AddCommand(serveCmd)
}
