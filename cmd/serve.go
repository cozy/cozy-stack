package cmd

import (
	"errors"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/web"
	"github.com/spf13/cobra"
)

var flagAllowRoot bool

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Starts the stack and listens for HTTP calls",
	Long: `Starts the stack and listens for HTTP calls
It will accept HTTP requests on localhost:8080 by default.
Use the --port and --host flags to change the listening option.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !flagAllowRoot && os.Getuid() == 0 {
			log.Errorf("Use --allow-root if you really want to start with the root user")
			return errors.New("Starting cozy-stack serve as root not allowed")
		}
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
	serveCmd.Flags().BoolVar(&flagAllowRoot, "allow-root", false, "Allow to start as root (disabled by default)")
}
