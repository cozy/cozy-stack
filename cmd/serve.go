package cmd

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"

	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/web"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the stack and listens for HTTP calls",
	Long: `Start the HTTP server for the server.
It will accept HTTP request on port 8080 by default.
If you want to use another port, on you can use the PORT env variable.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := Configure(); err != nil {
			return err
		}

		router := getGin()
		web.SetupRoutes(router)

		address := ":" + strconv.Itoa(config.GetConfig().Port)
		return router.Run(address)
	},
}

func init() {
	RootCmd.AddCommand(serveCmd)
}

func getGin() *gin.Engine {
	if config.GetConfig().Mode == config.Production {
		gin.SetMode(gin.ReleaseMode)
	}

	return gin.Default()
}
