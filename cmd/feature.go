package cmd

import (
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

func init() {
	featureCmdGroup.AddCommand(featureFlagCmd)
	RootCmd.AddCommand(featureCmdGroup)
}
