package cmd

import (
	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/web"
	"github.com/gin-gonic/gin"
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
		router := getGin()
		web.SetupRoutes(router)

		return router.Run(config.ServerAddr())
	},
}

func init() {
	RootCmd.AddCommand(serveCmd)
}

func getGin() *gin.Engine {
	if config.IsMode(config.Production) {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	engine.Use(gin.Logger())
	return engine
}
