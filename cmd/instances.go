package cmd

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/client"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/spf13/cobra"
)

var flagLocale string
var flagTimezone string
var flagEmail string
var flagApps []string
var flagDev bool

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
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
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
		c := newAdminClient()
		in, err := c.CreateInstance(&client.InstanceOptions{
			Domain:   domain,
			Apps:     flagApps,
			Locale:   flagLocale,
			Timezone: flagTimezone,
			Email:    flagEmail,
			Dev:      flagDev,
		})
		if err != nil {
			log.Errorf("Failed to create instance for domain %s", domain)
			return err
		}

		log.Infof("Instance created with success for domain %s", in.Attrs.Domain)
		log.Infof("Registration token: \"%s\"", hex.EncodeToString(in.Attrs.RegisterToken))
		log.Debugf("Instance created: %#v", in.Attrs)
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
		c := newAdminClient()
		list, err := c.ListInstances()
		if err != nil {
			return err
		}

		for _, i := range list {
			var dev string
			if i.Attrs.Dev {
				dev = "dev"
			} else {
				dev = "prod"
			}
			fmt.Printf("%s\t%s\t%s\n", i.Attrs.Domain, i.Attrs.StorageURL, dev)
		}

		return nil
	},
}

var destroyInstanceCmd = &cobra.Command{
	Use:   "destroy [domain]",
	Short: "Remove instance",
	Long: `
cozy-stack instances destroy allows to remove an instance
and all its data.
`,
	Aliases: []string{"rm"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}

		domain := args[0]

		reader := bufio.NewReader(os.Stdin)
		fmt.Printf(`Are you sure you want to remove instance for domain %s ?
All data associated with this domain will be permanently lost.
[yes/NO]: `, domain)

		str, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		str = strings.ToLower(strings.TrimSpace(str))
		if str != "yes" && str != "y" {
			return nil
		}

		fmt.Println()

		c := newAdminClient()
		in, err := c.DestroyInstance(domain)
		if err != nil {
			log.Errorf("Failed to remove instance for domain %s", domain)
			return err
		}

		fmt.Println()

		log.Infof("Instance for domain %s has been destroyed with success", in.Attrs.Domain)
		return nil
	},
}

var appTokenInstanceCmd = &cobra.Command{
	Use:   "token-app [domain] [slug]",
	Short: "Generate a new application token",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return cmd.Help()
		}
		c := newAdminClient()
		token, err := c.GetToken(&client.TokenOptions{
			Domain:   args[0],
			Subject:  args[1],
			Audience: "app",
		})
		if err != nil {
			return err
		}
		_, err = fmt.Println(token)
		return err
	},
}

var oauthTokenInstanceCmd = &cobra.Command{
	Use:   "token-oauth [domain] [clientid] [scopes]",
	Short: "Generate a new OAuth access token",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			return cmd.Help()
		}
		c := newAdminClient()
		token, err := c.GetToken(&client.TokenOptions{
			Domain:   args[0],
			Subject:  args[1],
			Audience: "access-token",
			Scope:    args[2:],
		})
		if err != nil {
			return err
		}
		_, err = fmt.Println(token)
		return err
	},
}

func init() {
	instanceCmdGroup.AddCommand(addInstanceCmd)
	instanceCmdGroup.AddCommand(lsInstanceCmd)
	instanceCmdGroup.AddCommand(destroyInstanceCmd)
	instanceCmdGroup.AddCommand(appTokenInstanceCmd)
	instanceCmdGroup.AddCommand(oauthTokenInstanceCmd)
	addInstanceCmd.Flags().StringVar(&flagLocale, "locale", instance.DefaultLocale, "Locale of the new cozy instance")
	addInstanceCmd.Flags().StringVar(&flagTimezone, "tz", "", "The timezone for the user")
	addInstanceCmd.Flags().StringVar(&flagEmail, "email", "", "The email of the owner")
	addInstanceCmd.Flags().StringSliceVar(&flagApps, "apps", nil, "Apps to be preinstalled")
	addInstanceCmd.Flags().BoolVar(&flagDev, "dev", false, "To create a development instance")
	RootCmd.AddCommand(instanceCmdGroup)
}
