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
	Use:   "install [--domain domain] [slug] [sourceurl]",
	Short: "Install an application with the specified slug name from the given source URL.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return cmd.Help()
		}
		if flagAppsDomain == "" {
			log.Error(errAppsMissingDomain)
			return cmd.Help()
		}
		c := newClient(flagAppsDomain, consts.Apps)
		app, err := c.InstallApp(&client.AppOptions{
			Slug:      args[0],
			SourceURL: args[1],
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
	Use:   "update [--domain domain] [slug]",
	Short: "Update the application with the specified slug name.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return cmd.Help()
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

func init() {
	appsCmdGroup.PersistentFlags().StringVar(&flagAppsDomain, "domain", "", "specify the domain name of the instance")

	appsCmdGroup.AddCommand(installAppCmd)
	appsCmdGroup.AddCommand(updateAppCmd)

	RootCmd.AddCommand(appsCmdGroup)
}
