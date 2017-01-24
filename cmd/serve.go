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
		return web.ListenAndServe()
	},
}

func init() {
	RootCmd.AddCommand(serveCmd)
}
