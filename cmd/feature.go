package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/spf13/cobra"
)

var featureCmdGroup = &cobra.Command{
	Use:   "feature <command>",
	Short: "Manage the feature flags",
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
		var obj map[string]interface{}
		if err = json.NewDecoder(res.Body).Decode(&obj); err != nil {
			return err
		}
		for k, v := range obj {
			fmt.Printf("- %s: %v\n", k, v)
		}
		return nil
	},
}

var featureSetCmd = &cobra.Command{
	Use:   "sets",
	Short: `Display and update the feature sets for an instance`,
	Long: `
cozy-stack feature sets displays the feature sets coming from the manager.

It can also take a list of sets that will replace the previous list (no merge).

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

func init() {
	featureFlagCmd.Flags().StringVar(&flagDomain, "domain", "", "Specify the domain name of the instance")
	featureSetCmd.Flags().StringVar(&flagDomain, "domain", "", "Specify the domain name of the instance")

	featureCmdGroup.AddCommand(featureFlagCmd)
	featureCmdGroup.AddCommand(featureSetCmd)
	RootCmd.AddCommand(featureCmdGroup)
}
