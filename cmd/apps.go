package cmd

// filesCmdGroup represents the instances command
import (
	"encoding/json"
	"errors"
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/spf13/cobra"
)

var errAppsMissingDomain = errors.New("Missing --domain flag")

var flagAppsDomain string
var flagAllDomains bool

var appsCmdGroup = &cobra.Command{
	Use:   "apps [command]",
	Short: "Interact with the cozy applications",
	Long: `
cozy-stack apps allows to interact with the cozy applications.

It provides commands to install or update applications from
a cozy.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var installAppCmd = &cobra.Command{
	Use:     "install [slug] [sourceurl]",
	Short:   "Install an application with the specified slug name from the given source URL.",
	Example: "$ cozy-stack apps install --domain cozy.tools:8080 files 'git://github.com/cozy-files-v3.git#build'",
	RunE: func(cmd *cobra.Command, args []string) error {
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
				c := newClient(in.Attrs.Domain, consts.Apps)
				_, err := c.InstallApp(&client.AppOptions{
					Slug:      slug,
					SourceURL: source,
				})
				if err != nil {
					if err.Error() == "Application with same slug already exists" {
						return nil
					}
					return err
				}
				log.Infof("Application installed successfully on %s", in.Attrs.Domain)
				return nil
			})
		}
		if flagAppsDomain == "" {
			log.Error(errAppsMissingDomain)
			return cmd.Help()
		}
		c := newClient(flagAppsDomain, consts.Apps)
		app, err := c.InstallApp(&client.AppOptions{
			Slug:      slug,
			SourceURL: source,
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
	},
}

var updateAppCmd = &cobra.Command{
	Use:     "update [slug]",
	Short:   "Update the application with the specified slug name.",
	Aliases: []string{"upgrade"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return cmd.Help()
		}
		if flagAllDomains {
			return foreachDomains(func(in *client.Instance) error {
				c := newClient(in.Attrs.Domain, consts.Apps)
				_, err := c.UpdateApp(&client.AppOptions{Slug: args[0]})
				if err != nil {
					if err.Error() == "Application is not installed" {
						return nil
					}
					return err
				}
				log.Infof("Application updated successfully on %s", in.Attrs.Domain)
				return nil
			})
		}
		if flagAppsDomain == "" {
			log.Error(errAppsMissingDomain)
			return cmd.Help()
		}
		c := newClient(flagAppsDomain, consts.Apps)
		app, err := c.UpdateApp(&client.AppOptions{Slug: args[0]})
		if err != nil {
			return err
		}
		json, err := json.MarshalIndent(app.Attrs, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(json))
		return nil
	},
}

var uninstallAppCmd = &cobra.Command{
	Use:     "uninstall [slug]",
	Short:   "Uninstall the application with the specified slug name.",
	Aliases: []string{"rm"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return cmd.Help()
		}
		if flagAppsDomain == "" {
			log.Error(errAppsMissingDomain)
			return cmd.Help()
		}
		c := newClient(flagAppsDomain, consts.Apps)
		app, err := c.UninstallApp(&client.AppOptions{Slug: args[0]})
		if err != nil {
			return err
		}
		json, err := json.MarshalIndent(app.Attrs, "", "  ")
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
			log.Warnf("%s: %s", i.Attrs.Domain, err)
			hasErr = true
		}
	}
	if hasErr {
		return errors.New("At least one error occured while executing this command")
	}
	return nil
}

func init() {
	appsCmdGroup.PersistentFlags().StringVar(&flagAppsDomain, "domain", "", "specify the domain name of the instance")
	appsCmdGroup.PersistentFlags().BoolVar(&flagAllDomains, "all-domains", false, "work on all domains iterativelly")

	appsCmdGroup.AddCommand(installAppCmd)
	appsCmdGroup.AddCommand(updateAppCmd)
	appsCmdGroup.AddCommand(uninstallAppCmd)

	RootCmd.AddCommand(appsCmdGroup)
}
