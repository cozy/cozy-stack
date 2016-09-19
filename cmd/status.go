package cmd

import (
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if the HTTP server is running",
	Long:  `Check if the HTTP server has been started and answer 200 for /status.`,
	Run: func(cmd *cobra.Command, args []string) {
		port := os.Getenv("PORT")
		if len(port) == 0 {
			port = "8080"
		}
		resp, err := http.Get("http://localhost:" + port + "/status")
		if err != nil {
			fmt.Println("Error the HTTP server is not running:", err)
			os.Exit(1)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			fmt.Println("Error, unexpected HTTP status code:", resp.Status)
			os.Exit(1)
		}
		fmt.Println("OK, the HTTP server is ready.")
	},
}

func init() {
	RootCmd.AddCommand(statusCmd)
}
