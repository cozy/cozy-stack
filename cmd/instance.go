package cmd

import (
	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/instance"
	"github.com/spf13/cobra"
)

var flagLocale string
var flagApps []string

// serveCmd represents the serve command
var instanceCmdGroup = &cobra.Command{
	Use:   "instances [command]",
	Short: "Manage instances of a stack",
	Long: `
cozy-stack instance allow to manage the instances of this stack

An instance is a logical space owned by one user and identified by a domain.
For example, bob.cozycloud.cc is the instance of Bob. A single cozy-stack
process can manage several instances.

Each instance has a separate space for storing files and a prefix used to
create its CouchDB databases.
	`,
	Run: func(cmd *cobra.Command, args []string) { cmd.Help() },
}

var addInstanceCmd = &cobra.Command{
	Use:   "add [domain]",
	Short: "Manage instances of a stack",
	Long: `
cozy-stack instance add allows to create an instance on the cozy for a
given domain.
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := Configure(); err != nil {
			return err
		}

		if len(args) == 0 {
			return cmd.Help()
		}

		domain := args[0]

		instance, err := instance.Create(domain, flagLocale, flagApps)
		if err != nil {
			return err
		}

		log.Infof("Instance created for domain %s", domain)
		log.Debugf("> %v", instance)
		return nil
	},
}

func init() {
	instanceCmdGroup.AddCommand(addInstanceCmd)
	addInstanceCmd.Flags().StringVar(&flagLocale, "locale", "en", "Locale of the new cozy instance")
	addInstanceCmd.Flags().StringSliceVar(&flagApps, "apps", nil, "Apps to be preinstalled")
	RootCmd.AddCommand(instanceCmdGroup)
}
