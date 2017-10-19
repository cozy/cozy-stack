package cmd

// filesCmdGroup represents the instances command
import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/spf13/cobra"
)

var errAppsMissingDomain = errors.New("Missing --domain flag, or COZY_DOMAIN env variable")

var flagAppsDomain string
var flagAllDomains bool
var flagAppsDeactivated bool

var flagKonnectorAccountID string
var flagKonnectorFolder string
var flagKonnectorsParameters string

var webappsCmdGroup = &cobra.Command{
	Use:   "apps [command]",
	Short: "Interact with the cozy applications",
	Long: `
cozy-stack apps allows to interact with the cozy applications.

It provides commands to install or update applications on
a cozy.
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Usage()
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

var showWebappCmd = &cobra.Command{
	Use:   "show [slug]",
	Short: "Show the application attributes",
	RunE: func(cmd *cobra.Command, args []string) error {
		return showApp(cmd, args, consts.Apps)
	},
}

var showKonnectorCmd = &cobra.Command{
	Use:   "show [slug]",
	Short: "Show the application attributes",
	RunE: func(cmd *cobra.Command, args []string) error {
		return showApp(cmd, args, consts.Konnectors)
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
		return cmd.Usage()
	},
}

var installKonnectorCmd = &cobra.Command{
	Use: "install [slug] [sourceurl]",
	Short: `Install a konnector with the specified slug name
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

var runKonnectorsCmd = &cobra.Command{
	Use:   "run [slug]",
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
			consts.Files,
			consts.Accounts,
		)

		var folderID, accountID string

		if flagKonnectorAccountID != "" && flagKonnectorFolder != "" {
			accountID = flagKonnectorAccountID
			d, err := c.GetDirByPath(flagKonnectorFolder)
			if err != nil {
				return err
			}
			folderID = d.ID
		} else {
			res, err := c.Req(&request.Options{
				Method: "GET",
				Path:   fmt.Sprintf("/data/%s/_all_docs", url.PathEscape(consts.Accounts)),
				Queries: url.Values{
					"include_docs": []string{"true"},
				},
			})
			if err != nil {
				return err
			}
			defer res.Body.Close()

			var allDocs struct {
				Rows []struct {
					ID  string          `json:"id"`
					Doc json.RawMessage `json:"doc"`
				} `json:"rows"`
			}

			if err = json.NewDecoder(res.Body).Decode(&allDocs); err != nil {
				return err
			}

			for _, r := range allDocs.Rows {
				var v struct {
					ID          string `json:"_id"`
					AccountType string `json:"account_type"`
					FolderID    string `json:"folderId"`
				}
				if strings.HasPrefix(r.ID, "_design") {
					continue
				}
				if err = json.Unmarshal(r.Doc, &v); err != nil {
					return err
				}
				if v.AccountType == slug {
					folderID = v.FolderID
					accountID = v.ID
					break
				}
			}
		}
		if accountID == "" {
			return fmt.Errorf("Could not find account associated with konnector %q", slug)
		}

		j, err := c.JobPush(&client.JobOptions{
			Worker: "konnector",
			Arguments: map[string]interface{}{
				"konnector":      slug,
				"account":        accountID,
				"folder_to_save": folderID,
			},
		})
		if err != nil {
			return err
		}
		job, err := json.MarshalIndent(j, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(job))
		return nil
	},
}

func installApp(cmd *cobra.Command, args []string, appType string) error {
	if len(args) < 1 {
		return cmd.Usage()
	}
	slug := args[0]
	var source string
	if len(args) == 1 {
		s, ok := consts.AppsRegistry[slug]
		if !ok {
			return cmd.Usage()
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
	if domain == "" && config.IsDevRelease() {
		domain = "cozy.tools:8080"
	}

	webappsCmdGroup.PersistentFlags().StringVar(&flagAppsDomain, "domain", domain, "specify the domain name of the instance")
	webappsCmdGroup.PersistentFlags().BoolVar(&flagAllDomains, "all-domains", false, "work on all domains iterativelly")

	installWebappCmd.PersistentFlags().BoolVar(&flagAppsDeactivated, "ask-permissions", false, "specify that the application should not be activated after installation")

	runKonnectorsCmd.PersistentFlags().StringVar(&flagKonnectorAccountID, "account-id", "", "specify the account ID to use for running the konnector")
	runKonnectorsCmd.PersistentFlags().StringVar(&flagKonnectorFolder, "folder", "", "specify the folder path associated with the konnector")

	webappsCmdGroup.AddCommand(lsWebappsCmd)
	webappsCmdGroup.AddCommand(showWebappCmd)
	webappsCmdGroup.AddCommand(installWebappCmd)
	webappsCmdGroup.AddCommand(updateWebappCmd)
	webappsCmdGroup.AddCommand(uninstallWebappCmd)

	konnectorsCmdGroup.PersistentFlags().StringVar(&flagAppsDomain, "domain", domain, "specify the domain name of the instance")
	konnectorsCmdGroup.PersistentFlags().StringVar(&flagKonnectorsParameters, "parameters", "", "override the parameters of the installed konnector")
	konnectorsCmdGroup.PersistentFlags().BoolVar(&flagAllDomains, "all-domains", false, "work on all domains iterativelly")

	konnectorsCmdGroup.AddCommand(lsKonnectorsCmd)
	konnectorsCmdGroup.AddCommand(showKonnectorCmd)
	konnectorsCmdGroup.AddCommand(installKonnectorCmd)
	konnectorsCmdGroup.AddCommand(updateKonnectorCmd)
	konnectorsCmdGroup.AddCommand(uninstallKonnectorCmd)
	konnectorsCmdGroup.AddCommand(runKonnectorsCmd)

	RootCmd.AddCommand(webappsCmdGroup)
	RootCmd.AddCommand(konnectorsCmdGroup)
}
