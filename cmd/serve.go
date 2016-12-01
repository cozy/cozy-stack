package cmd

import (
	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/web"
	"github.com/cozy/cozy-stack/web/apps"
	"github.com/cozy/cozy-stack/web/router"
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
		r, err := web.Create(&web.Config{
			Router:    router.Setup(echo.New()),
			Domain:    config.GetConfig().Domain,
			Assets:    config.GetConfig().Assets,
			ServeApps: apps.Serve,
		})
		if err != nil {
			return err
		}
		return r.Start(config.ServerAddr())
	},
}

func init() {
	RootCmd.AddCommand(serveCmd)
}
