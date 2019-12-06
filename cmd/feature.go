package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/spf13/cobra"
)

var flagWithSources bool

var featureCmdGroup = &cobra.Command{
	Use:     "features <command>",
	Aliases: []string{"feature"},
	Short:   "Manage the feature flags",
}

var featureShowCmd = &cobra.Command{
	Use:   "show",
	Short: `Display the computed feature flags for an instance`,
	Long: `
cozy-stack feature show displays the feature flags that are shown by apps.
`,
	Example: `$ cozy-stack feature show --domain cozy.tools:8080`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagDomain == "" {
			errPrintfln("%s", errMissingDomain)
			return cmd.Usage()
		}
		c := newClient(flagDomain, consts.Settings)
		req := &request.Options{
			Method: "GET",
			Path:   "/settings/flags",
		}
		if flagWithSources {
			req.Queries = url.Values{"include": {"source"}}
		}
		res, err := c.Req(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		var obj struct {
			Data struct {
				Attributes map[string]json.RawMessage `json:"attributes"`
			} `json:"data"`
			Included []struct {
				ID         string                     `json:"id"`
				Attributes map[string]json.RawMessage `json:"attributes"`
			} `json:"included"`
		}
		if err = json.NewDecoder(res.Body).Decode(&obj); err != nil {
			return err
		}
		for k, v := range obj.Data.Attributes {
			fmt.Printf("- %s: %s\n", k, string(v))
		}
		if len(obj.Included) > 0 {
			fmt.Printf("\nSources:\n")
			for _, source := range obj.Included {
				fmt.Printf("- %s\n", source.ID)
				for k, v := range source.Attributes {
					fmt.Printf("\t- %s: %s\n", k, string(v))
				}
			}
		}
		return nil
	},
}

var featureFlagCmd = &cobra.Command{
	Use:   "flags",
	Short: `Display and update the feature flags for an instance`,
	Long: `
cozy-stack feature flags displays the feature flags that are specific to an instance.

It can also take a list of flags to update.

If you give a null value, the flag will be removed.
`,
	Example: `$ cozy-stack feature flags --domain cozy.tools:8080 '{"add_this_flag": true, "remove_this_flag": null}'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagDomain == "" {
			errPrintfln("%s", errMissingDomain)
			return cmd.Usage()
		}
		c := newAdminClient()
		req := request.Options{
			Method: "GET",
			Path:   fmt.Sprintf("/instances/%s/feature/flags", flagDomain),
		}
		if len(args) > 0 {
			req.Method = "PATCH"
			req.Body = strings.NewReader(args[0])
		}
		res, err := c.Req(&req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		var obj map[string]json.RawMessage
		if err = json.NewDecoder(res.Body).Decode(&obj); err != nil {
			return err
		}
		for k, v := range obj {
			fmt.Printf("- %s: %s\n", k, string(v))
		}
		return nil
	},
}

var featureSetCmd = &cobra.Command{
	Use:   "sets",
	Short: `Display and update the feature sets for an instance`,
	Long: `
cozy-stack feature sets displays the feature sets coming from the manager.

It can also take a space-separated list of sets that will replace the previous
list (no merge).

All the sets can be removed by setting an empty list ('').
`,
	Example: `$ cozy-stack feature sets --domain cozy.tools:8080 'set1 set2'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagDomain == "" {
			errPrintfln("%s", errMissingDomain)
			return cmd.Usage()
		}
		c := newAdminClient()
		req := request.Options{
			Method: "GET",
			Path:   fmt.Sprintf("/instances/%s/feature/sets", flagDomain),
		}
		if len(args) > 0 {
			list := args
			if len(args) == 1 {
				list = strings.Fields(args[0])
			}
			buf, err := json.Marshal(list)
			if err != nil {
				return err
			}
			req.Method = "PUT"
			req.Body = bytes.NewReader(buf)
		}
		res, err := c.Req(&req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		var sets []string
		if err = json.NewDecoder(res.Body).Decode(&sets); err != nil {
			return err
		}
		for _, set := range sets {
			fmt.Printf("- %v\n", set)
		}
		return nil
	},
}

var featureContextCmd = &cobra.Command{
	Use:   "context <context-name>",
	Short: `Display and update the feature flags for a context`,
	Long: `
cozy-stack feature context displays the feature flags for a context.

It can also create, update, or remove flags (with a ratio and value).

To remove a flag, set it to an empty array (or null).
`,
	Example: `$ cozy-stack feature context --context beta '{"set_this_flag": [{"ratio": 0.1, "value": 1}, {"ratio": 0.9, "value": 2}] }'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagContext == "" {
			return cmd.Usage()
		}
		c := newAdminClient()
		req := request.Options{
			Method: "GET",
			Path:   fmt.Sprintf("/instances/feature/contexts/%s", flagContext),
		}
		if len(args) > 0 {
			req.Method = "PATCH"
			req.Body = strings.NewReader(args[0])
		}
		res, err := c.Req(&req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		var obj map[string]json.RawMessage
		if err = json.NewDecoder(res.Body).Decode(&obj); err != nil {
			return err
		}
		for k, v := range obj {
			fmt.Printf("- %s: %s\n", k, string(v))
		}
		return nil
	},
}

var featureDefaultCmd = &cobra.Command{
	Use:   "defaults",
	Short: `Display and update the default values for feature flags`,
	Long: `
cozy-stack feature defaults displays the default values for feature flags.

It can also take a list of flags to update.

If you give a null value, the flag will be removed.
`,
	Example: `$ cozy-stack feature defaults '{"add_this_flag": true, "remove_this_flag": null}'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newAdminClient()
		req := request.Options{
			Method: "GET",
			Path:   "/instances/feature/defaults",
		}
		if len(args) > 0 {
			req.Method = "PATCH"
			req.Body = strings.NewReader(args[0])
		}
		res, err := c.Req(&req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		var obj map[string]json.RawMessage
		if err = json.NewDecoder(res.Body).Decode(&obj); err != nil {
			return err
		}
		for k, v := range obj {
			fmt.Printf("- %s: %s\n", k, string(v))
		}
		return nil
	},
}

func init() {
	featureShowCmd.Flags().StringVar(&flagDomain, "domain", "", "Specify the domain name of the instance")
	featureShowCmd.Flags().BoolVar(&flagWithSources, "source", false, "Show the sources of the feature flags")
	featureFlagCmd.Flags().StringVar(&flagDomain, "domain", "", "Specify the domain name of the instance")
	featureSetCmd.Flags().StringVar(&flagDomain, "domain", "", "Specify the domain name of the instance")
	featureContextCmd.Flags().StringVar(&flagContext, "context", "", "The context for the feature flags")

	featureCmdGroup.AddCommand(featureShowCmd)
	featureCmdGroup.AddCommand(featureFlagCmd)
	featureCmdGroup.AddCommand(featureSetCmd)
	featureCmdGroup.AddCommand(featureContextCmd)
	featureCmdGroup.AddCommand(featureDefaultCmd)
	RootCmd.AddCommand(featureCmdGroup)
}
