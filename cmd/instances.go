package cmd

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	humanize "github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
)

var flagLocale string
var flagTimezone string
var flagEmail string
var flagPublicName string
var flagSettings string
var flagDiskQuota string
var flagApps []string
var flagDev bool
var flagPassphrase string
var flagForce bool
var flagExpire time.Duration

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

var cleanInstanceCmd = &cobra.Command{
	Use:   "clean [domain]",
	Short: "Clean badly removed instances",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}

		domain := args[0]
		i := couchdb.SimpleDatabasePrefix(domain)
		return couchdb.DeleteAllDBs(i)
	},
}

var showInstanceCmd = &cobra.Command{
	Use:   "show [domain]",
	Short: "Show the instance of the specified domain",
	Long: `
cozy-stack instances show allows to show the instance on the cozy for a
given domain.
`,
	Example: "$ cozy-stack instances show cozy.tools:8080",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		domain := args[0]
		c := newAdminClient()
		in, err := c.GetInstance(domain)
		if err != nil {
			return err
		}
		json, err := json.MarshalIndent(in, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(json))
		return nil
	},
}

var addInstanceCmd = &cobra.Command{
	Use:   "add [domain]",
	Short: "Manage instances of a stack",
	Long: `
cozy-stack instances add allows to create an instance on the cozy for a
given domain.
`,
	Example: "$ cozy-stack instances add --dev --passphrase cozy --apps drive,photos,settings cozy.tools:8080",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}

		var diskQuota uint64
		if flagDiskQuota != "" {
			var err error
			diskQuota, err = humanize.ParseBytes(flagDiskQuota)
			if err != nil {
				return err
			}
		}

		domain := args[0]
		c := newAdminClient()
		in, err := c.CreateInstance(&client.InstanceOptions{
			Domain:     domain,
			Apps:       flagApps,
			Locale:     flagLocale,
			Timezone:   flagTimezone,
			Email:      flagEmail,
			PublicName: flagPublicName,
			Settings:   flagSettings,
			DiskQuota:  int64(diskQuota),
			Dev:        flagDev,
			Passphrase: flagPassphrase,
		})
		if err != nil {
			errPrintfln(
				"Failed to create instance for domain %s", domain)
			return err
		}

		fmt.Printf("Instance created with success for domain %s\n", in.Attrs.Domain)
		if in.Attrs.RegisterToken != nil {
			fmt.Printf("Registration token: \"%s\"\n", hex.EncodeToString(in.Attrs.RegisterToken))
		}
		return nil
	},
}

var quotaInstanceCmd = &cobra.Command{
	Use:   "set-disk-quota [domain] [disk-quota]",
	Short: "Change the disk-quota of the instance",
	Long: `
cozy-stack instances set-disk-quota allows to change the disk-quota of the
instance of the given domain. Set the quota to 0 to remove the quota.
`,
	Example: "$ cozy-stack instances set-disk-quota cozy.tools:8080 3GB",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return cmd.Help()
		}
		diskQuota, err := humanize.ParseBytes(args[1])
		if err != nil {
			return fmt.Errorf("Could not parse disk-quota: %s", err)
		}
		domain := args[0]
		c := newAdminClient()
		_, err = c.ModifyInstance(domain, &client.InstanceOptions{
			DiskQuota: int64(diskQuota),
		})
		return err
	},
}

var debugInstanceCmd = &cobra.Command{
	Use:   "debug [domain] [true/false]",
	Short: "Activate or deactivate debugging of the instance",
	Long: `
cozy-stack instances debug allows to activate or deactivate the debugging of a
specific domain.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return cmd.Help()
		}
		domain := args[0]
		debug, err := strconv.ParseBool(args[1])
		if err != nil {
			return err
		}
		c := newAdminClient()
		_, err = c.ModifyInstance(domain, &client.InstanceOptions{
			Debug: &debug,
		})
		return err
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
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, i := range list {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\tv%d\n",
				i.Attrs.Domain,
				i.Attrs.Locale,
				formatSize(i.Attrs.BytesDiskQuota),
				formatDev(i.Attrs.Dev),
				formatOnboarded(len(i.Attrs.RegisterToken) == 0),
				i.Attrs.IndexViewsVersion,
			)
		}
		return w.Flush()
	},
}

func formatSize(size int64) string {
	if size == 0 {
		return "unlimited"
	}
	return humanize.Bytes(uint64(size))
}

func formatDev(dev bool) string {
	if dev {
		return "dev"
	}
	return "prod"
}

func formatOnboarded(onboarded bool) string {
	if onboarded {
		return "onboarded"
	}
	return "pending"
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

		if !flagForce {
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
		}

		c := newAdminClient()
		err := c.DestroyInstance(domain)
		if err != nil {
			errPrintfln(
				"An error occured while destroying instance for domain %s", domain)
			return err
		}

		fmt.Printf("Instance for domain %s has been destroyed with success\n", domain)
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
			Expire:   flagExpire,
		})
		if err != nil {
			return err
		}
		_, err = fmt.Println(token)
		return err
	},
}

var cliTokenInstanceCmd = &cobra.Command{
	Use:   "token-cli [domain] [scopes]",
	Short: "Generate a new CLI access token (global access)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return cmd.Help()
		}
		c := newAdminClient()
		token, err := c.GetToken(&client.TokenOptions{
			Domain:   args[0],
			Scope:    args[1:],
			Audience: "cli",
			Expire:   flagExpire,
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
			Expire:   flagExpire,
		})
		if err != nil {
			return err
		}
		_, err = fmt.Println(token)
		return err
	},
}

var oauthClientInstanceCmd = &cobra.Command{
	Use:   "client-oauth [domain] [redirect_uri] [client_name] [software_id]",
	Short: "Register a new OAuth client",
	Long:  `It registers a new OAuth client and returns its client_id`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 4 {
			return cmd.Help()
		}
		c := newAdminClient()
		clientID, err := c.RegisterOAuthClient(&client.OAuthClientOptions{
			Domain:      args[0],
			RedirectURI: args[1],
			ClientName:  args[2],
			SoftwareID:  args[3],
		})
		if err != nil {
			return err
		}
		_, err = fmt.Println(clientID)
		return err
	},
}

func init() {
	instanceCmdGroup.AddCommand(showInstanceCmd)
	instanceCmdGroup.AddCommand(addInstanceCmd)
	instanceCmdGroup.AddCommand(cleanInstanceCmd)
	instanceCmdGroup.AddCommand(lsInstanceCmd)
	instanceCmdGroup.AddCommand(quotaInstanceCmd)
	instanceCmdGroup.AddCommand(debugInstanceCmd)
	instanceCmdGroup.AddCommand(destroyInstanceCmd)
	instanceCmdGroup.AddCommand(appTokenInstanceCmd)
	instanceCmdGroup.AddCommand(cliTokenInstanceCmd)
	instanceCmdGroup.AddCommand(oauthTokenInstanceCmd)
	instanceCmdGroup.AddCommand(oauthClientInstanceCmd)
	addInstanceCmd.Flags().StringVar(&flagLocale, "locale", instance.DefaultLocale, "Locale of the new cozy instance")
	addInstanceCmd.Flags().StringVar(&flagTimezone, "tz", "", "The timezone for the user")
	addInstanceCmd.Flags().StringVar(&flagEmail, "email", "", "The email of the owner")
	addInstanceCmd.Flags().StringVar(&flagPublicName, "public-name", "", "The public name of the owner")
	addInstanceCmd.Flags().StringVar(&flagSettings, "settings", "", "A list of settings (eg context:foo,offer:premium)")
	addInstanceCmd.Flags().StringVar(&flagDiskQuota, "disk-quota", "", "The quota allowed to the instance's VFS")
	addInstanceCmd.Flags().StringSliceVar(&flagApps, "apps", nil, "Apps to be preinstalled")
	addInstanceCmd.Flags().BoolVar(&flagDev, "dev", false, "To create a development instance")
	addInstanceCmd.Flags().StringVar(&flagPassphrase, "passphrase", "", "Register the instance with this passphrase (useful for tests)")
	destroyInstanceCmd.Flags().BoolVar(&flagForce, "force", false, "Force the deletion without asking for confirmation")
	appTokenInstanceCmd.Flags().DurationVar(&flagExpire, "expire", 0, "Make the token expires in this amount of time")
	oauthTokenInstanceCmd.Flags().DurationVar(&flagExpire, "expire", 0, "Make the token expires in this amount of time")
	RootCmd.AddCommand(instanceCmdGroup)
}
