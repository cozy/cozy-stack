package cmd

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/instance"
	humanize "github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
)

var flagDomain string
var flagDomainAliases []string
var flagListFields []string
var flagLocale string
var flagTimezone string
var flagEmail string
var flagPublicName string
var flagSettings string
var flagDiskQuota string
var flagApps []string
var flagBlocked bool
var flagDev bool
var flagPassphrase string
var flagForce bool
var flagFsckDry bool
var flagFsckPrune bool
var flagJSON bool
var flagDirectory string
var flagIncreaseQuota bool
var flagForceRegistry bool
var flagOnlyRegistry bool
var flagSwiftCluster int
var flagUUID string
var flagTOSSigned string
var flagTOS string
var flagTOSLatest string
var flagContextName string
var flagOnboardingFinished bool
var flagExpire time.Duration

// instanceCmdGroup represents the instances command
var instanceCmdGroup = &cobra.Command{
	Use:   "instances <command>",
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
		return cmd.Usage()
	},
}

var showInstanceCmd = &cobra.Command{
	Use:   "show <domain>",
	Short: "Show the instance of the specified domain",
	Long: `
cozy-stack instances show allows to show the instance on the cozy for a
given domain.
`,
	Example: "$ cozy-stack instances show cozy.tools:8080",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Usage()
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

var showPrefixInstanceCmd = &cobra.Command{
	Use:   "show-prefix <domain>",
	Short: "Show the instance prefix of the specified domain",
	Long: `
cozy-stack instances show allows to show the instance prefix on the cozy for a
given domain. The prefix is used for databases and VFS prefixing.
`,
	Example: "$ cozy-stack instances show-prefix cozy.tools:8080",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Usage()
		}
		domain := args[0]
		c := newAdminClient()
		in, err := c.GetInstance(domain)
		if err != nil {
			return err
		}
		if in.Attrs.Prefix != "" {
			fmt.Println(in.Attrs.Prefix)
		} else {
			fmt.Println(in.Attrs.Domain)
		}
		return nil
	},
}

var addInstanceCmd = &cobra.Command{
	Use:   "add <domain>",
	Short: "Manage instances of a stack",
	Long: `
cozy-stack instances add allows to create an instance on the cozy for a
given domain.

If the COZY_DISABLE_INSTANCES_ADD_RM env variable is set, creating and
destroying instances will be desactivated and the content of this variable will
be used as the error message.
`,
	Example: "$ cozy-stack instances add --dev --passphrase cozy --apps drive,photos,settings cozy.tools:8080",
	RunE: func(cmd *cobra.Command, args []string) error {
		if reason := os.Getenv("COZY_DISABLE_INSTANCES_ADD_RM"); reason != "" {
			return fmt.Errorf("Sorry, instances add is disabled: %s", reason)
		}
		if len(args) == 0 {
			return cmd.Usage()
		}

		var diskQuota int64
		if flagDiskQuota != "" {
			diskQuotaU, err := humanize.ParseBytes(flagDiskQuota)
			if err != nil {
				return err
			}
			diskQuota = int64(diskQuotaU)
		}

		domain := args[0]
		c := newAdminClient()
		in, err := c.CreateInstance(&client.InstanceOptions{
			Domain:        domain,
			DomainAliases: flagDomainAliases,
			Locale:        flagLocale,
			UUID:          flagUUID,
			TOSSigned:     flagTOSSigned,
			Timezone:      flagTimezone,
			ContextName:   flagContextName,
			Email:         flagEmail,
			PublicName:    flagPublicName,
			Settings:      flagSettings,
			SwiftCluster:  flagSwiftCluster,
			DiskQuota:     diskQuota,
			Apps:          flagApps,
			Passphrase:    flagPassphrase,
			Dev:           flagDev,
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
		if len(flagApps) == 0 {
			return nil
		}
		apps, err := newClient(domain, consts.Apps).ListApps(consts.Apps)
		if err == nil && len(flagApps) != len(apps) {
			for _, slug := range flagApps {
				found := false
				for _, app := range apps {
					if app.Attrs.Slug == slug {
						found = true
						break
					}
				}
				if !found {
					fmt.Printf("/!\\ Application %s has not been installed\n", slug)
				}
			}
		}
		return nil
	},
}

var modifyInstanceCmd = &cobra.Command{
	Use:     "modify <domain>",
	Aliases: []string{"update"},
	Short:   "Modify the instance properties",
	Long: `
cozy-stack instances modify allows to change the instance properties and
settings for a specified domain.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Usage()
		}

		var diskQuota int64
		if flagDiskQuota != "" {
			diskQuotaU, err := humanize.ParseBytes(flagDiskQuota)
			if err != nil {
				return err
			}
			diskQuota = int64(diskQuotaU)
		}

		domain := args[0]
		c := newAdminClient()
		opts := &client.InstanceOptions{
			Domain:        domain,
			DomainAliases: flagDomainAliases,
			Locale:        flagLocale,
			UUID:          flagUUID,
			TOSSigned:     flagTOS,
			TOSLatest:     flagTOSLatest,
			Timezone:      flagTimezone,
			ContextName:   flagContextName,
			Email:         flagEmail,
			PublicName:    flagPublicName,
			Settings:      flagSettings,
			SwiftCluster:  flagSwiftCluster,
			DiskQuota:     diskQuota,
		}
		if flag := cmd.Flag("blocked"); flag.Changed {
			opts.Blocked = &flagBlocked
		}
		if flagOnboardingFinished {
			opts.OnboardingFinished = &flagOnboardingFinished
		}
		in, err := c.ModifyInstance(opts)
		if err != nil {
			errPrintfln(
				"Failed to modify instance for domain %s", domain)
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

var quotaInstanceCmd = &cobra.Command{
	Use:   "set-disk-quota <domain> <disk-quota>",
	Short: "Change the disk-quota of the instance",
	Long: `
cozy-stack instances set-disk-quota allows to change the disk-quota of the
instance of the given domain. Set the quota to 0 to remove the quota.
`,
	Example: "$ cozy-stack instances set-disk-quota cozy.tools:8080 3GB",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return cmd.Usage()
		}
		diskQuota, err := humanize.ParseBytes(args[1])
		if err != nil {
			return fmt.Errorf("Could not parse disk-quota: %s", err)
		}
		domain := args[0]
		c := newAdminClient()
		_, err = c.ModifyInstance(&client.InstanceOptions{
			Domain:    domain,
			DiskQuota: int64(diskQuota),
		})
		return err
	},
}

var debugInstanceCmd = &cobra.Command{
	Use:   "debug <domain> <true/false>",
	Short: "Activate or deactivate debugging of the instance",
	Long: `
cozy-stack instances debug allows to activate or deactivate the debugging of a
specific domain.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return cmd.Usage()
		}
		domain := args[0]
		debug, err := strconv.ParseBool(args[1])
		if err != nil {
			return err
		}
		c := newAdminClient()
		_, err = c.ModifyInstance(&client.InstanceOptions{
			Domain: domain,
			Debug:  &debug,
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
		if flagJSON {
			if len(flagListFields) > 0 {
				for _, inst := range list {
					var values []interface{}
					values, err = extractFields(inst.Attrs, flagListFields)
					if err != nil {
						return err
					}
					m := make(map[string]interface{}, len(flagListFields))
					for i, fieldName := range flagListFields {
						m[fieldName] = values[i]
					}
					if err = json.NewEncoder(os.Stdout).Encode(m); err != nil {
						return err
					}
				}
			} else {
				for _, inst := range list {
					if err = json.NewEncoder(os.Stdout).Encode(inst.Attrs); err != nil {
						return err
					}
				}
			}
		} else {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			if len(flagListFields) > 0 {
				format := strings.Repeat("%v\t", len(flagListFields))
				format = format[:len(format)-1] + "\n"
				for _, inst := range list {
					var values []interface{}
					values, err = extractFields(inst.Attrs, flagListFields)
					if err != nil {
						return err
					}
					fmt.Fprintf(w, format, values...)
				}
			} else {
				for _, i := range list {
					prefix := i.Attrs.Prefix
					if prefix == "" {
						prefix = i.Attrs.Domain
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\tv%d\t%s\n",
						i.Attrs.Domain,
						i.Attrs.Locale,
						formatSize(i.Attrs.BytesDiskQuota),
						formatDev(i.Attrs.Dev),
						formatOnboarded(i),
						i.Attrs.IndexViewsVersion,
						prefix,
					)
				}
			}
			w.Flush()
		}
		return nil
	},
}

func extractFields(data interface{}, fieldsNames []string) (values []interface{}, err error) {
	var m map[string]interface{}
	var b []byte
	b, err = json.Marshal(data)
	if err != nil {
		return
	}
	if err = json.Unmarshal(b, &m); err != nil {
		return
	}
	values = make([]interface{}, len(fieldsNames))
	for i, fieldName := range fieldsNames {
		if v, ok := m[fieldName]; ok {
			values[i] = v
		}
	}
	return
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

func formatOnboarded(i *client.Instance) string {
	if i.Attrs.OnboardingFinished {
		return "onboarded"
	}
	if len(i.Attrs.RegisterToken) > 0 {
		return "onboarding"
	}
	return "pending"
}

var destroyInstanceCmd = &cobra.Command{
	Use:   "destroy <domain>",
	Short: "Remove instance",
	Long: `
cozy-stack instances destroy allows to remove an instance
and all its data.
`,
	Aliases: []string{"rm"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if reason := os.Getenv("COZY_DISABLE_INSTANCES_ADD_RM"); reason != "" {
			return fmt.Errorf("Sorry, instances add is disabled: %s", reason)
		}
		if len(args) == 0 {
			return cmd.Usage()
		}

		domain := args[0]

		if !flagForce {
			reader := bufio.NewReader(os.Stdin)
			fmt.Printf(`Are you sure you want to remove instance for domain %s?
All data associated with this domain will be permanently lost.
Type again the domain to confirm: `, domain)

			str, err := reader.ReadString('\n')
			if err != nil {
				return err
			}

			str = strings.ToLower(strings.TrimSpace(str))
			if str != domain {
				return errors.New("Aborted")
			}

			fmt.Println()
		}

		c := newAdminClient()
		err := c.DestroyInstance(domain)
		if err != nil {
			errPrintfln(
				"An error occurred while destroying instance for domain %s", domain)
			return err
		}

		fmt.Printf("Instance for domain %s has been destroyed with success\n", domain)
		return nil
	},
}

var fsckInstanceCmd = &cobra.Command{
	Use:   "fsck <domain>",
	Short: "Check and repair a vfs",
	Long: `
The cozy-stack fsck command checks that the files in the VFS are not
desynchronized, ie a file present in CouchDB but not swift/localfs, or present
in swift/localfs but not couchdb.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Usage()
		}

		domain := args[0]

		c := newAdminClient()
		list, err := c.FsckInstance(domain, flagFsckPrune, flagFsckDry)
		if err != nil {
			return err
		}

		if len(list) == 0 {
			fmt.Printf("Instance for domain %s is clean\n", domain)
		} else {
			for _, entry := range list {
				fmt.Printf("- %q: %s\n", entry["filename"], entry["message"])
				if pruneAction := entry["prune_action"]; pruneAction != "" {
					fmt.Printf("  %s...", pruneAction)
					if pruneError := entry["prune_error"]; pruneError != "" {
						fmt.Printf("error: %s\n", pruneError)
					} else {
						fmt.Println("ok")
					}
				}
			}
		}
		return nil
	},
}

func appOrKonnectorTokenInstance(cmd *cobra.Command, args []string, appType string) error {
	if len(args) < 2 {
		return cmd.Usage()
	}
	c := newAdminClient()
	token, err := c.GetToken(&client.TokenOptions{
		Domain:   args[0],
		Subject:  args[1],
		Audience: appType,
		Expire:   &flagExpire,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Println(token)
	return err
}

var appTokenInstanceCmd = &cobra.Command{
	Use:   "token-app <domain> <slug>",
	Short: "Generate a new application token",
	RunE: func(cmd *cobra.Command, args []string) error {
		return appOrKonnectorTokenInstance(cmd, args, "app")
	},
}

var konnectorTokenInstanceCmd = &cobra.Command{
	Use:   "token-konnector <domain> <slug>",
	Short: "Generate a new konnector token",
	RunE: func(cmd *cobra.Command, args []string) error {
		return appOrKonnectorTokenInstance(cmd, args, "konn")
	},
}

var cliTokenInstanceCmd = &cobra.Command{
	Use:   "token-cli <domain> <scopes>",
	Short: "Generate a new CLI access token (global access)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return cmd.Usage()
		}
		c := newAdminClient()
		token, err := c.GetToken(&client.TokenOptions{
			Domain:   args[0],
			Scope:    args[1:],
			Audience: "cli",
		})
		if err != nil {
			return err
		}
		_, err = fmt.Println(token)
		return err
	},
}

var oauthTokenInstanceCmd = &cobra.Command{
	Use:   "token-oauth <domain> <clientid> <scopes>",
	Short: "Generate a new OAuth access token",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			return cmd.Usage()
		}
		c := newAdminClient()
		token, err := c.GetToken(&client.TokenOptions{
			Domain:   args[0],
			Subject:  args[1],
			Audience: "access-token",
			Scope:    args[2:],
			Expire:   &flagExpire,
		})
		if err != nil {
			return err
		}
		_, err = fmt.Println(token)
		return err
	},
}

var oauthRefreshTokenInstanceCmd = &cobra.Command{
	Use:   "refresh-token-oauth <domain> <clientid> <scopes>",
	Short: "Generate a new OAuth refresh token",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 3 {
			return cmd.Usage()
		}
		c := newAdminClient()
		token, err := c.GetToken(&client.TokenOptions{
			Domain:   args[0],
			Subject:  args[1],
			Audience: "refresh-token",
			Scope:    args[2:],
		})
		if err != nil {
			return err
		}
		_, err = fmt.Println(token)
		return err
	},
}

var oauthClientInstanceCmd = &cobra.Command{
	Use:   "client-oauth <domain> <redirect_uri> <client_name> <software_id>",
	Short: "Register a new OAuth client",
	Long:  `It registers a new OAuth client and returns its client_id`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 4 {
			return cmd.Usage()
		}
		c := newAdminClient()
		oauthClient, err := c.RegisterOAuthClient(&client.OAuthClientOptions{
			Domain:      args[0],
			RedirectURI: args[1],
			ClientName:  args[2],
			SoftwareID:  args[3],
		})
		if err != nil {
			return err
		}
		if flagJSON {
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "\t")
			err = encoder.Encode(oauthClient)
		} else {
			_, err = fmt.Println(oauthClient["client_id"])
		}
		return err
	},
}

var updateCmd = &cobra.Command{
	Use:   "update [slugs...]",
	Short: "Start the updates for the specified domain instance.",
	Long: `Start the updates for the specified domain instance. Use whether the --domain
flag to specify the instance or the --all-domains flags to updates all domains.
The slugs arguments can be used to select which applications should be
updated.`,
	Aliases: []string{"updates"},
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newAdminClient()
		if flagAllDomains {
			logs := make(chan *client.JobLog)
			go func() {
				for log := range logs {
					fmt.Printf("[%s][time:%s]", log.Level, log.Time.Format(time.RFC3339))
					for k, v := range log.Data {
						fmt.Printf("[%s:%s]", k, v)
					}
					fmt.Printf(" %s\n", log.Message)
				}
			}()
			return c.Updates(&client.UpdatesOptions{
				Slugs:         args,
				ForceRegistry: flagForceRegistry,
				OnlyRegistry:  flagOnlyRegistry,
				Logs:          logs,
			})
		}
		if flagDomain == "" {
			return errAppsMissingDomain
		}
		return c.Updates(&client.UpdatesOptions{
			Domain:             flagDomain,
			DomainsWithContext: flagContextName,
			Slugs:              args,
			ForceRegistry:      flagForceRegistry,
			OnlyRegistry:       flagOnlyRegistry,
		})
	},
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export an instance to a tarball",
	Long:  `Export the files and photos albums to a tarball (.tar.gz)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newAdminClient()
		return c.Export(flagDomain)
	},
}

var importCmd = &cobra.Command{
	Use:   "import <tarball>",
	Short: "Import a tarball",
	Long:  `Import a tarball with files, photos albums and contacts to an instance`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newAdminClient()
		if len(args) < 1 {
			return errors.New("The path to the tarball is missing")
		}
		return c.Import(flagDomain, &client.ImportOptions{
			Filename:      args[0],
			Destination:   flagDirectory,
			IncreaseQuota: flagIncreaseQuota,
		})
	},
}

func init() {
	instanceCmdGroup.AddCommand(showInstanceCmd)
	instanceCmdGroup.AddCommand(showPrefixInstanceCmd)
	instanceCmdGroup.AddCommand(addInstanceCmd)
	instanceCmdGroup.AddCommand(modifyInstanceCmd)
	instanceCmdGroup.AddCommand(lsInstanceCmd)
	instanceCmdGroup.AddCommand(quotaInstanceCmd)
	instanceCmdGroup.AddCommand(debugInstanceCmd)
	instanceCmdGroup.AddCommand(destroyInstanceCmd)
	instanceCmdGroup.AddCommand(fsckInstanceCmd)
	instanceCmdGroup.AddCommand(appTokenInstanceCmd)
	instanceCmdGroup.AddCommand(konnectorTokenInstanceCmd)
	instanceCmdGroup.AddCommand(cliTokenInstanceCmd)
	instanceCmdGroup.AddCommand(oauthTokenInstanceCmd)
	instanceCmdGroup.AddCommand(oauthRefreshTokenInstanceCmd)
	instanceCmdGroup.AddCommand(oauthClientInstanceCmd)
	instanceCmdGroup.AddCommand(updateCmd)
	instanceCmdGroup.AddCommand(exportCmd)
	instanceCmdGroup.AddCommand(importCmd)
	addInstanceCmd.Flags().StringSliceVar(&flagDomainAliases, "domain-aliases", nil, "Specify one or more aliases domain for the instance (separated by ',')")
	addInstanceCmd.Flags().StringVar(&flagLocale, "locale", instance.DefaultLocale, "Locale of the new cozy instance")
	addInstanceCmd.Flags().StringVar(&flagUUID, "uuid", "", "The UUID of the instance")
	addInstanceCmd.Flags().StringVar(&flagTOS, "tos", "", "The TOS version signed")
	addInstanceCmd.Flags().StringVar(&flagTimezone, "tz", "", "The timezone for the user")
	addInstanceCmd.Flags().StringVar(&flagContextName, "context-name", "", "Context name of the instance")
	addInstanceCmd.Flags().StringVar(&flagEmail, "email", "", "The email of the owner")
	addInstanceCmd.Flags().StringVar(&flagPublicName, "public-name", "", "The public name of the owner")
	addInstanceCmd.Flags().StringVar(&flagSettings, "settings", "", "A list of settings (eg context:foo,offer:premium)")
	addInstanceCmd.Flags().IntVar(&flagSwiftCluster, "swift-cluster", 0, "Specify a cluster number for swift")
	addInstanceCmd.Flags().StringVar(&flagDiskQuota, "disk-quota", "", "The quota allowed to the instance's VFS")
	addInstanceCmd.Flags().StringSliceVar(&flagApps, "apps", nil, "Apps to be preinstalled")
	addInstanceCmd.Flags().BoolVar(&flagDev, "dev", false, "To create a development instance")
	addInstanceCmd.Flags().StringVar(&flagPassphrase, "passphrase", "", "Register the instance with this passphrase (useful for tests)")
	modifyInstanceCmd.Flags().StringSliceVar(&flagDomainAliases, "domain-aliases", nil, "Specify one or more aliases domain for the instance (separated by ',')")
	modifyInstanceCmd.Flags().StringVar(&flagLocale, "locale", instance.DefaultLocale, "New locale")
	modifyInstanceCmd.Flags().StringVar(&flagUUID, "uuid", "", "New UUID")
	modifyInstanceCmd.Flags().StringVar(&flagTOS, "tos", "", "Update the TOS version signed")
	modifyInstanceCmd.Flags().StringVar(&flagTOSLatest, "tos-latest", "", "Update the latest TOS version")
	modifyInstanceCmd.Flags().StringVar(&flagTimezone, "tz", "", "New timezone")
	modifyInstanceCmd.Flags().StringVar(&flagContextName, "context-name", "", "New context name")
	modifyInstanceCmd.Flags().StringVar(&flagEmail, "email", "", "New email")
	modifyInstanceCmd.Flags().StringVar(&flagPublicName, "public-name", "", "New public name")
	modifyInstanceCmd.Flags().StringVar(&flagSettings, "settings", "", "New list of settings (eg offer:premium)")
	modifyInstanceCmd.Flags().IntVar(&flagSwiftCluster, "swift-cluster", 0, "New swift cluster")
	modifyInstanceCmd.Flags().StringVar(&flagDiskQuota, "disk-quota", "", "Specify a new disk quota")
	modifyInstanceCmd.Flags().BoolVar(&flagBlocked, "blocked", false, "Block the instance")
	modifyInstanceCmd.Flags().BoolVar(&flagOnboardingFinished, "onboarding-finished", false, "Force the finishing of the onboarding")
	destroyInstanceCmd.Flags().BoolVar(&flagForce, "force", false, "Force the deletion without asking for confirmation")
	fsckInstanceCmd.Flags().BoolVar(&flagFsckDry, "dry", false, "Don't modify the VFS, only show the inconsistencies")
	fsckInstanceCmd.Flags().BoolVar(&flagFsckPrune, "prune", false, "Try to solve inconsistencies by modifying the file system")
	oauthClientInstanceCmd.Flags().BoolVar(&flagJSON, "json", false, "Output more informations in JSON format")
	oauthTokenInstanceCmd.Flags().DurationVar(&flagExpire, "expire", 0, "Make the token expires in this amount of time")
	appTokenInstanceCmd.Flags().DurationVar(&flagExpire, "expire", 0, "Make the token expires in this amount of time")
	lsInstanceCmd.Flags().BoolVar(&flagJSON, "json", false, "Show each line as a json representation of the instance")
	lsInstanceCmd.Flags().StringSliceVar(&flagListFields, "fields", nil, "Arguments shown for each line in the list")
	updateCmd.Flags().BoolVar(&flagAllDomains, "all-domains", false, "Work on all domains iterativelly")
	updateCmd.Flags().StringVar(&flagDomain, "domain", "", "Specify the domain name of the instance")
	updateCmd.Flags().StringVar(&flagContextName, "context-name", "", "Work only on the instances with the given context name")
	updateCmd.Flags().BoolVar(&flagForceRegistry, "force-registry", false, "Force to update all applications sources from git to the registry")
	updateCmd.Flags().BoolVar(&flagOnlyRegistry, "only-registry", false, "Only update applications installed from the registry")
	exportCmd.Flags().StringVar(&flagDomain, "domain", "", "Specify the domain name of the instance")
	importCmd.Flags().StringVar(&flagDomain, "domain", "", "Specify the domain name of the instance")
	importCmd.Flags().StringVar(&flagDirectory, "directory", "", "Put the imported files inside this directory")
	importCmd.Flags().BoolVar(&flagIncreaseQuota, "increase-quota", false, "Increase the disk quota if needed for importing all the files")
	exportCmd.MarkFlagRequired("domain")
	importCmd.MarkFlagRequired("domain")
	RootCmd.AddCommand(instanceCmdGroup)
}
