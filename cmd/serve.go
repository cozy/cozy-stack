package cmd

import (
	"fmt"

	"github.com/cozy/cozy-stack/web"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the stack and listens for HTTP calls",
	Long: `Start the HTTP server for the server.
It will accept HTTP request on port 8080 by default.
If you want to use another port, on you can use the PORT env variable.`,
	Run: func(cmd *cobra.Command, args []string) {
		router := gin.Default()
		web.SetupRoutes(router)
		if err := router.Run(); err != nil {
			fmt.Println("Error:", err)
		}
	},
}

func init() {
	RootCmd.AddCommand(serveCmd)
}
