package cmd

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the stack and listens for HTTP calls",
	Long: `Start the HTTP server for the server.
It will accept HTTP request.`,
	Run: func(cmd *cobra.Command, args []string) {
		r := gin.Default()
		r.GET("/ping", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"message": "pong",
			})
		})
		if err := r.Run(); err != nil {
			fmt.Println("Error:", err)
		}
	},
}

func init() {
	RootCmd.AddCommand(serveCmd)
}
