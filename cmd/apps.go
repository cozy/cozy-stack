package cmd

// filesCmdGroup represents the instances command
import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/spf13/cobra"
)

var errAppsMissingDomain = errors.New("Missing --domain flag, or COZY_DOMAIN env variable")

var flagAppsDomain string
var flagAllDomains bool
var flagAppsDeactivated bool

var webappsCmdGroup = &cobra.Command{
	Use:   "apps [command]",
	Short: "Interact with the cozy applications",
	Long: `
cozy-stack apps allows to interact with the cozy applications.

It provides commands to install or update applications on
a cozy.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var installWebappCmd = &cobra.Command{
	Use: "install [slug] [sourceurl]",
	Short: `Install an application with the specified slug name
from the given source URL.`,
	Example: "$ cozy-stack apps install --domain cozy.tools:8080 drive 'git://github.com/cozy/cozy-drive.git#build-drive'",
	RunE: func(cmd *cobra.Command, args []string) error {
		return installApp(cmd, args, consts.Apps)
	},
}

var updateWebappCmd = &cobra.Command{
	Use:     "update [slug] [sourceurl]",
	Short:   "Update the application with the specified slug name.",
	Aliases: []string{"upgrade"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return updateApp(cmd, args, consts.Apps)
	},
}

var uninstallWebappCmd = &cobra.Command{
	Use:     "uninstall [slug]",
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

var konnectorsCmdGroup = &cobra.Command{
	Use:   "konnectors [command]",
	Short: "Interact with the cozy applications",
	Long: `
cozy-stack konnectors allows to interact with the cozy konnectors.

It provides commands to install or update applications on
a cozy.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var installKonnectorCmd = &cobra.Command{
	Use: "install [slug] [sourceurl]",
	Short: `Install an konnector with the specified slug name
from the given source URL.`,
	Example: "$ cozy-stack konnectors install --domain cozy.tools:8080 trainline 'git://github.com/cozy/cozy-konnector-trainline.git#build'",
	RunE: func(cmd *cobra.Command, args []string) error {
		return installApp(cmd, args, consts.Konnectors)
	},
}

var updateKonnectorCmd = &cobra.Command{
	Use:     "update [slug] [sourceurl]",
	Short:   "Update the konnector with the specified slug name.",
	Aliases: []string{"upgrade"},
	RunE: func(cmd *cobra.Command, args []string) error {
		return updateApp(cmd, args, consts.Konnectors)
	},
}

var uninstallKonnectorCmd = &cobra.Command{
	Use:     "uninstall [slug]",
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

func installApp(cmd *cobra.Command, args []string, appType string) error {
	if len(args) < 1 {
		return cmd.Help()
	}
	slug := args[0]
	var source string
	if len(args) == 1 {
		s, ok := consts.AppsRegistry[slug]
		if !ok {
			return cmd.Help()
		}
		source = s
	} else {
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
		return cmd.Help()
	}
	c := newClient(flagAppsDomain, appType)
	app, err := c.InstallApp(&client.AppOptions{
		AppType:     appType,
		Slug:        slug,
		SourceURL:   source,
		Deactivated: flagAppsDeactivated,
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
		return cmd.Help()
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
			})
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
		return cmd.Help()
	}
	c := newClient(flagAppsDomain, appType)
	app, err := c.UpdateApp(&client.AppOptions{
		AppType:   appType,
		Slug:      args[0],
		SourceURL: src,
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

func uninstallApp(cmd *cobra.Command, args []string, appType string) error {
	if len(args) != 1 {
		return cmd.Help()
	}
	if flagAppsDomain == "" {
		errPrintfln("%s", errAppsMissingDomain)
		return cmd.Help()
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

func lsApps(cmd *cobra.Command, args []string, appType string) error {
	if flagAppsDomain == "" {
		errPrintfln("%s", errAppsMissingDomain)
		return cmd.Help()
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
		return errors.New("At least one error occured while executing this command")
	}
	return nil
}

func init() {
	domain := os.Getenv("COZY_DOMAIN")
	webappsCmdGroup.PersistentFlags().StringVar(&flagAppsDomain, "domain", domain, "specify the domain name of the instance")
	webappsCmdGroup.PersistentFlags().BoolVar(&flagAllDomains, "all-domains", false, "work on all domains iterativelly")
	installWebappCmd.PersistentFlags().BoolVar(&flagAppsDeactivated, "ask-permissions", false, "specify that the application should not be activated after installation")

	webappsCmdGroup.AddCommand(lsWebappsCmd)
	webappsCmdGroup.AddCommand(installWebappCmd)
	webappsCmdGroup.AddCommand(updateWebappCmd)
	webappsCmdGroup.AddCommand(uninstallWebappCmd)

	konnectorsCmdGroup.PersistentFlags().StringVar(&flagAppsDomain, "domain", domain, "specify the domain name of the instance")
	konnectorsCmdGroup.PersistentFlags().BoolVar(&flagAllDomains, "all-domains", false, "work on all domains iterativelly")

	konnectorsCmdGroup.AddCommand(lsKonnectorsCmd)
	konnectorsCmdGroup.AddCommand(installKonnectorCmd)
	konnectorsCmdGroup.AddCommand(updateKonnectorCmd)
	konnectorsCmdGroup.AddCommand(uninstallKonnectorCmd)

	RootCmd.AddCommand(webappsCmdGroup)
	RootCmd.AddCommand(konnectorsCmdGroup)
}
