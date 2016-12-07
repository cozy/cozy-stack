package cmd

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/instance"
	"github.com/spf13/cobra"
)

var flagLocale string
var flagApps []string

// instanceCmdGroup represents the instances command
var instanceCmdGroup = &cobra.Command{
	Use:   "instances [command]",
	Short: "Manage instances of a stack",
	Long: `
cozy-stack instances allows to manage the instances of this stack

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
cozy-stack instances add allows to create an instance on the cozy for a
given domain.
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}

		domain := args[0]

		i, err := instance.Create(domain, flagLocale, flagApps)
		if err != nil {
			log.Errorf("Error while creating instance for domain %s", domain)
			return err
		}

		params := url.Values{
			"registerToken": {hex.EncodeToString(i.RegisterToken)},
		}

		log.Infof("Instance created with success for domain %s", i.Domain)
		log.Infof("Owner registration link : onboarding.%s/?%s", i.Addr(), params.Encode())
		log.Debugf("Instance created: %#v", i)
		return nil
	},
}

var lsInstanceCmd = &cobra.Command{
	Use:   "ls",
	Short: "List instances",
	Long: `
cozy-stack instances ls allows to list all the instances that can be served
by this server.
	`,
	RunE: func(cmd *cobra.Command, args []string) error {
		instances, err := instance.List()
		if err != nil {
			return err
		}

		if len(instances) == 0 {
			log.Warnf("No instances")
			return nil
		}

		for _, i := range instances {
			fmt.Printf("instance: %s\tdomain: %s\tstorage: %s\n", i.DocID, i.Domain, i.StorageURL)
		}

		return nil
	},
}

var destroyInstanceCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Remove instance",
	Long: ` cozy-stack instances destroy allows to remove an instance
and all its data.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf(`
Are you sure you want to remove instance for domain %s ?
All data associated with this domain will be permanently lost.
[yes/NO]: `, domain)

		in, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		if strings.ToLower(strings.TrimSpace(in)) != "yes" {
			return nil
		}

		fmt.Println()

		instance, err := instance.Destroy(domain)
		if err != nil {
			log.Errorf("Error while removing instance for domain %s", domain)
			return err
		}

		log.Infof("Instance for domain %s has been destroyed with success", instance.Domain)
		return nil
	},
}

func init() {
	instanceCmdGroup.AddCommand(addInstanceCmd)
	instanceCmdGroup.AddCommand(lsInstanceCmd)
	instanceCmdGroup.AddCommand(destroyInstanceCmd)
	addInstanceCmd.Flags().StringVar(&flagLocale, "locale", "en", "Locale of the new cozy instance")
	addInstanceCmd.Flags().StringSliceVar(&flagApps, "apps", nil, "Apps to be preinstalled")
	RootCmd.AddCommand(instanceCmdGroup)
}
