package cmd

import (
	"github.com/cozy/cozy-stack/web"
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
		return web.ListenAndServe()
	},
}

func init() {
	RootCmd.AddCommand(serveCmd)
}
