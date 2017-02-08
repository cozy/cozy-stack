package cmd

import (
	"github.com/cozy/cozy-stack/pkg/instance"
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
		if err := instance.StartJobs(); err != nil {
			return err
		}
		return web.ListenAndServe()
	},
}

var serveAppDir = &cobra.Command{
	Use:   "serve-appdir [directory]",
	Short: "Starts the stack along with a ",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return cmd.Help()
		}
		if err := instance.StartJobs(); err != nil {
			return err
		}
		return web.ListenAndServeWithAppDir(args[0])
	},
}

func init() {
	RootCmd.AddCommand(serveCmd)
	RootCmd.AddCommand(serveAppDir)
}
