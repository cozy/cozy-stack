package cmd

import (
	"github.com/cozy/cozy-stack/pkg/couchdb"
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
		if err := startJobs(); err != nil {
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
		if err := startJobs(); err != nil {
			return err
		}
		return web.ListenAndServeWithAppDir(args[0])
	},
}

func startJobs() error {
	// The serve method starts all the jobs systems associated with the created
	// instances.
	//
	// TODO: on distributed stacks, we should not have to iterate over all
	// instances on each startup
	ins, err := instance.List()
	if err != nil && !couchdb.IsNoDatabaseError(err) {
		return err
	}
	for _, in := range ins {
		if err := in.StartJobSystem(); err != nil {
			return err
		}
	}
	return nil
}

func init() {
	RootCmd.AddCommand(serveCmd)
	RootCmd.AddCommand(serveAppDir)
}
