package cmd

// filesCmdGroup represents the instances command
import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/spf13/cobra"
)

var errAppsMissingDomain = errors.New("Missing --domain flag, or COZY_DOMAIN env variable")

var flagAppsDomain string
var flagAllDomains bool
var flagAppsDeactivated bool
var flagSafeUpdate bool

var flagKonnectorAccountID string
var flagKonnectorsParameters string

var webappsCmdGroup = &cobra.Command{
	Use:   "apps <command>",
	Short: "Interact with the applications",
	Long: `
cozy-stack apps allows to interact with the cozy applications.

It provides commands to install or update applications on
a cozy.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Usage()
	},
}

var triggersCmdGroup = &cobra.Command{
	Use:   "triggers <command>",
	Short: "Interact with the triggers",
	Long: `
cozy-stack apps allows to interact with the cozy triggers.

It provides command to run a specific trigger.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Usage()
	},
}

var installWebappCmd = &cobra.Command{
	Use: "install <slug> [sourceurl]",
	Short: `Install an application with the specified slug name
from the given source URL.`,
	Example: "$ cozy-stack apps install --domain cozy.tools:8080 drive registry://drive/stable",
	Long:    "[Some schemes](https://docs.cozy.io/en/cozy-stack/apps/#sources) are allowed as `[sourceurl]`.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return installApp(cmd, args, consts.Apps)
	},
}

var updateWebappCmd = &cobra.Command{
	Use:     "update <slug> [sourceurl]",
	Short:   "Update the application with the specified slug name.",
	Aliases: []string{"upgrade"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return updateApp(cmd, args, consts.Apps)
	},
}

var uninstallWebappCmd = &cobra.Command{
	Use:     "uninstall <slug>",
	Short:   "Uninstall the application with the specified slug name.",
	Aliases: []string{"rm"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return uninstallApp(cmd, args, consts.Apps)
	},
}

var lsWebappsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List the installed applications.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return lsApps(cmd, args, consts.Apps)
	},
}

var showWebappCmd = &cobra.Command{
	Use:   "show <slug>",
	Short: "Show the application attributes",
	RunE: func(cmd *cobra.Command, args []string) error {
		return showApp(cmd, args, consts.Apps)
	},
}

var showWebappTriggersCmd = &cobra.Command{
	Use:   "show-from-app <slug>",
	Short: "Show the application triggers",
	RunE: func(cmd *cobra.Command, args []string) error {
		return showWebAppTriggers(cmd, args, consts.Apps)
	},
}

var showKonnectorCmd = &cobra.Command{
	Use:   "show <slug>",
	Short: "Show the application attributes",
	RunE: func(cmd *cobra.Command, args []string) error {
		return showApp(cmd, args, consts.Konnectors)
	},
}

var konnectorsCmdGroup = &cobra.Command{
	Use:   "konnectors <command>",
	Short: "Interact with the konnectors",
	Long: `
cozy-stack konnectors allows to interact with the cozy konnectors.

It provides commands to install or update applications on
a cozy.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Usage()
	},
}

var installKonnectorCmd = &cobra.Command{
	Use: "install <slug> [sourceurl]",
	Short: `Install a konnector with the specified slug name
from the given source URL.`,
	Long: `
Install a konnector with the specified slug name. You can also provide the
version number to install a specific release if you use the registry:// scheme.
Following formats are accepted:
	registry://<konnector>/<channel>/<version>
	registry://<konnector>/<channel>
	registry://<konnector>/<version>
	registry://<konnector>

If you provide a channel and a version, the channel is ignored.
Default channel is stable.
Default version is latest.
`,
	Example: `
$ cozy-stack konnectors install --domain cozy.tools:8080 trainline registry://trainline/stable/1.0.1
$ cozy-stack konnectors install --domain cozy.tools:8080 trainline registry://trainline/stable
$ cozy-stack konnectors install --domain cozy.tools:8080 trainline registry://trainline/1.2.0
$ cozy-stack konnectors install --domain cozy.tools:8080 trainline registry://trainline
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return installApp(cmd, args, consts.Konnectors)
	},
}

var updateKonnectorCmd = &cobra.Command{
	Use:     "update <slug> [sourceurl]",
	Short:   "Update the konnector with the specified slug name.",
	Aliases: []string{"upgrade"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return updateApp(cmd, args, consts.Konnectors)
	},
}

var uninstallKonnectorCmd = &cobra.Command{
	Use:     "uninstall <slug>",
	Short:   "Uninstall the konnector with the specified slug name.",
	Aliases: []string{"rm"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return uninstallApp(cmd, args, consts.Konnectors)
	},
}

var lsKonnectorsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List the installed konnectors.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return lsApps(cmd, args, consts.Konnectors)
	},
}

var runKonnectorsCmd = &cobra.Command{
	Use:   "run <slug>",
	Short: "Run a konnector.",
	Long:  "Run a konnector named with specified slug using the specified options.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagAppsDomain == "" {
			errPrintfln("%s", errAppsMissingDomain)
			return cmd.Usage()
		}
		if len(args) < 1 {
			return cmd.Usage()
		}

		slug := args[0]
		c := newClient(flagAppsDomain,
			consts.Jobs+":POST:konnector:worker",
			consts.Triggers,
			consts.Files,
			consts.Accounts,
		)

		ts, err := c.GetTriggers("konnector")
		if err != nil {
			return err
		}

		type localTrigger struct {
			id        string
			accountID string
		}

		var triggers []*localTrigger
		for _, t := range ts {
			var msg struct {
				Slug    string `json:"konnector"`
				Account string `json:"account"`
			}
			if err = json.Unmarshal(t.Attrs.Message, &msg); err != nil {
				return err
			}
			if msg.Slug == slug {
				triggers = append(triggers, &localTrigger{t.ID, msg.Account})
			}
		}

		if len(triggers) == 0 {
			return fmt.Errorf("Could not find a konnector %q: "+
				"it may be installed but it is not activated (no related trigger)", slug)
		}

		var trigger *localTrigger
		if len(triggers) > 1 || flagKonnectorAccountID != "" {
			if flagKonnectorAccountID == "" {
				// TODO: better multi-account support
				return errors.New("Found multiple konnectors with different accounts:" +
					"use the --account-id flag (support for better multi-accounts support in the CLI is still a work in progress)")
			}
			for _, t := range triggers {
				if t.accountID == flagKonnectorAccountID {
					trigger = t
					break
				}
			}
			if trigger == nil {
				return fmt.Errorf("Could not find konnector linked to account with id %q",
					flagKonnectorAccountID)
			}
		} else {
			trigger = triggers[0]
		}

		j, err := c.TriggerLaunch(trigger.id)
		if err != nil {
			return err
		}

		json, err := json.MarshalIndent(j, "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(json))
		return nil
	},
}

func installApp(cmd *cobra.Command, args []string, appType string) error {
	if len(args) < 1 {
		return cmd.Usage()
	}
	slug := args[0]
	source := "registry://" + slug + "/stable"
	if len(args) > 1 {
		source = args[1]
	}
	if flagAllDomains {
		return foreachDomains(func(in *client.Instance) error {
			c := newClient(in.Attrs.Domain, appType)
			_, err := c.InstallApp(&client.AppOptions{
				AppType:     appType,
				Slug:        slug,
				SourceURL:   source,
				Deactivated: flagAppsDeactivated,
			})
			if err != nil {
				if err.Error() == "Application with same slug already exists" {
					return nil
				}
				return err
			}
			fmt.Printf("Application installed successfully on %s\n", in.Attrs.Domain)
			return nil
		})
	}
	if flagAppsDomain == "" {
		errPrintfln("%s", errAppsMissingDomain)
		return cmd.Usage()
	}

	var overridenParameters *json.RawMessage
	if flagKonnectorsParameters != "" {
		tmp := json.RawMessage(flagKonnectorsParameters)
		overridenParameters = &tmp
	}

	c := newClient(flagAppsDomain, appType)
	app, err := c.InstallApp(&client.AppOptions{
		AppType:     appType,
		Slug:        slug,
		SourceURL:   source,
		Deactivated: flagAppsDeactivated,

		OverridenParameters: overridenParameters,
	})
	if err != nil {
		return err
	}
	json, err := json.MarshalIndent(app.Attrs, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(json))
	return nil
}

func updateApp(cmd *cobra.Command, args []string, appType string) error {
	if len(args) == 0 || len(args) > 2 {
		return cmd.Usage()
	}
	var src string
	if len(args) > 1 {
		src = args[1]
	}
	if flagAllDomains {
		return foreachDomains(func(in *client.Instance) error {
			c := newClient(in.Attrs.Domain, appType)
			_, err := c.UpdateApp(&client.AppOptions{
				AppType:   appType,
				Slug:      args[0],
				SourceURL: src,
			}, flagSafeUpdate)
			if err != nil {
				if err.Error() == "Application is not installed" {
					return nil
				}
				return err
			}
			fmt.Printf("Application updated successfully on %s\n", in.Attrs.Domain)
			return nil
		})
	}
	if flagAppsDomain == "" {
		errPrintfln("%s", errAppsMissingDomain)
		return cmd.Usage()
	}

	var overridenParameters *json.RawMessage
	if flagKonnectorsParameters != "" {
		tmp := json.RawMessage(flagKonnectorsParameters)
		overridenParameters = &tmp
	}

	c := newClient(flagAppsDomain, appType)
	app, err := c.UpdateApp(&client.AppOptions{
		AppType:   appType,
		Slug:      args[0],
		SourceURL: src,

		OverridenParameters: overridenParameters,
	}, flagSafeUpdate)
	if err != nil {
		return err
	}
	json, err := json.MarshalIndent(app.Attrs, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(json))
	return nil
}

func uninstallApp(cmd *cobra.Command, args []string, appType string) error {
	if len(args) != 1 {
		return cmd.Usage()
	}
	if flagAppsDomain == "" {
		errPrintfln("%s", errAppsMissingDomain)
		return cmd.Usage()
	}
	c := newClient(flagAppsDomain, appType)
	app, err := c.UninstallApp(&client.AppOptions{
		AppType: appType,
		Slug:    args[0],
	})
	if err != nil {
		return err
	}
	json, err := json.MarshalIndent(app.Attrs, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(json))
	return nil
}

func showApp(cmd *cobra.Command, args []string, appType string) error {
	if flagAppsDomain == "" {
		errPrintfln("%s", errAppsMissingDomain)
		return cmd.Usage()
	}
	if len(args) < 1 {
		return cmd.Usage()
	}
	c := newClient(flagAppsDomain, appType)
	app, err := c.GetApp(&client.AppOptions{
		Slug:    args[0],
		AppType: appType,
	})
	if err != nil {
		return err
	}
	json, err := json.MarshalIndent(app.Attrs, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(json))
	return nil
}

func showWebAppTriggers(cmd *cobra.Command, args []string, appType string) error {
	if flagAppsDomain == "" {
		errPrintfln("%s", errAppsMissingDomain)
		return cmd.Usage()
	}
	if len(args) < 1 {
		return cmd.Usage()
	}
	c := newClient(flagAppsDomain, appType, consts.Triggers)
	app, err := c.GetApp(&client.AppOptions{
		Slug:    args[0],
		AppType: appType,
	})

	if err != nil {
		return err
	}

	var triggerIDs []string
	if app.Attrs.Services == nil {
		fmt.Printf("No triggers\n")
		return nil
	}
	for _, service := range *app.Attrs.Services {
		triggerIDs = append(triggerIDs, service.TriggerID)
	}
	var triggers []*client.Trigger
	var trigger *client.Trigger
	for _, triggerID := range triggerIDs {
		trigger, err = c.GetTrigger(triggerID)
		if err != nil {
			return err
		}

		triggers = append(triggers, trigger)
	}
	json, err := json.MarshalIndent(triggers, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(json))
	return nil
}

var listTriggerCmd = &cobra.Command{
	Use:     "ls",
	Short:   `List triggers`,
	Example: "$ cozy-stack triggers ls --domain cozy.tools:8080",
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagAppsDomain == "" {
			errPrintfln("%s", errAppsMissingDomain)
			return cmd.Usage()
		}
		c := newClient(flagAppsDomain, consts.Triggers)
		list, err := c.ListTriggers()
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, t := range list {
			fmt.Fprintf(w, "%s\t%s\t%s\t%v\t%s\n",
				t.ID,
				t.Attrs.WorkerType,
				t.Attrs.Type,
				t.Attrs.Arguments,
				t.Attrs.Debounce,
			)
		}
		return w.Flush()
	},
}

var launchTriggerCmd = &cobra.Command{
	Use:     "launch [triggerId]",
	Short:   `Creates a job from a specific trigger`,
	Example: "$ cozy-stack triggers launch --domain cozy.tools:8080 748f42b65aca8c99ec2492eb660d1891",
	RunE: func(cmd *cobra.Command, args []string) error {
		return launchTrigger(cmd, args)
	},
}

func launchTrigger(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return cmd.Usage()
	}
	if flagAppsDomain == "" {
		errPrintfln("%s", errAppsMissingDomain)
		return cmd.Usage()
	}

	// Creates client
	c := newClient(flagAppsDomain, consts.Triggers)

	// Creates job
	j, err := c.TriggerLaunch(args[0])
	if err != nil {
		return err
	}

	// Print JSON
	json, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(json))
	return nil
}

func lsApps(cmd *cobra.Command, args []string, appType string) error {
	if flagAppsDomain == "" {
		errPrintfln("%s", errAppsMissingDomain)
		return cmd.Usage()
	}
	c := newClient(flagAppsDomain, appType)
	// TODO(pagination)
	apps, err := c.ListApps(appType)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, app := range apps {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			app.Attrs.Slug,
			app.Attrs.Source,
			app.Attrs.Version,
			app.Attrs.State,
		)
	}
	return w.Flush()
}

var appsVersionsCmd = &cobra.Command{
	Use:     "versions",
	Short:   `Show apps versions of all instances`,
	Example: "$ cozy-stack apps versions",
	RunE: func(cmd *cobra.Command, args []string) error {
		instances, err := instance.List()
		if err != nil {
			return err
		}
		counter := make(map[string]map[string]int)

		for _, instance := range instances {
			var apps []*app.WebappManifest
			req := &couchdb.AllDocsRequest{Limit: 100}
			err := couchdb.GetAllDocs(instance, consts.Apps, req, &apps)
			if err != nil {
				return err
			}

			for _, app := range apps {
				if _, ok := counter[app.Slug()]; !ok {
					counter[app.Slug()] = map[string]int{app.Version(): 0}
				}
				counter[app.Slug()][app.Version()]++
			}
		}
		json, err := json.MarshalIndent(counter, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(json))
		return nil
	},
}

func foreachDomains(predicate func(*client.Instance) error) error {
	c := newAdminClient()
	// TODO(pagination): Make this iteration more robust
	list, err := c.ListInstances()
	if err != nil {
		return nil
	}
	var hasErr bool
	for _, i := range list {
		if err = predicate(i); err != nil {
			errPrintfln("%s: %s", i.Attrs.Domain, err)
			hasErr = true
		}
	}
	if hasErr {
		return errors.New("At least one error occurred while executing this command")
	}
	return nil
}

func init() {
	domain := os.Getenv("COZY_DOMAIN")
	if domain == "" && build.IsDevRelease() {
		domain = defaultDevDomain
	}

	webappsCmdGroup.PersistentFlags().StringVar(&flagAppsDomain, "domain", domain, "specify the domain name of the instance")
	webappsCmdGroup.PersistentFlags().BoolVar(&flagAllDomains, "all-domains", false, "work on all domains iterativelly")

	installWebappCmd.PersistentFlags().BoolVar(&flagAppsDeactivated, "ask-permissions", false, "specify that the application should not be activated after installation")
	updateWebappCmd.PersistentFlags().BoolVar(&flagSafeUpdate, "safe", false, "do not upgrade if there are blocking changes")
	updateKonnectorCmd.PersistentFlags().BoolVar(&flagSafeUpdate, "safe", false, "do not upgrade if there are blocking changes")

	runKonnectorsCmd.PersistentFlags().StringVar(&flagKonnectorAccountID, "account-id", "", "specify the account ID to use for running the konnector")

	triggersCmdGroup.PersistentFlags().StringVar(&flagAppsDomain, "domain", domain, "specify the domain name of the instance")
	triggersCmdGroup.AddCommand(launchTriggerCmd)
	triggersCmdGroup.AddCommand(listTriggerCmd)
	triggersCmdGroup.AddCommand(showWebappTriggersCmd)

	webappsCmdGroup.AddCommand(lsWebappsCmd)
	webappsCmdGroup.AddCommand(showWebappCmd)
	webappsCmdGroup.AddCommand(installWebappCmd)
	webappsCmdGroup.AddCommand(updateWebappCmd)
	webappsCmdGroup.AddCommand(uninstallWebappCmd)
	webappsCmdGroup.AddCommand(appsVersionsCmd)

	konnectorsCmdGroup.PersistentFlags().StringVar(&flagAppsDomain, "domain", domain, "specify the domain name of the instance")
	konnectorsCmdGroup.PersistentFlags().StringVar(&flagKonnectorsParameters, "parameters", "", "override the parameters of the installed konnector")
	konnectorsCmdGroup.PersistentFlags().BoolVar(&flagAllDomains, "all-domains", false, "work on all domains iterativelly")

	konnectorsCmdGroup.AddCommand(lsKonnectorsCmd)
	konnectorsCmdGroup.AddCommand(showKonnectorCmd)
	konnectorsCmdGroup.AddCommand(installKonnectorCmd)
	konnectorsCmdGroup.AddCommand(updateKonnectorCmd)
	konnectorsCmdGroup.AddCommand(uninstallKonnectorCmd)
	konnectorsCmdGroup.AddCommand(runKonnectorsCmd)

	RootCmd.AddCommand(triggersCmdGroup)
	RootCmd.AddCommand(webappsCmdGroup)
	RootCmd.AddCommand(konnectorsCmdGroup)
}
