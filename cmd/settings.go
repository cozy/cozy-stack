package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/spf13/cobra"
)

var errSettingsMissingDomain = errors.New("Missing --domain flag")

var flagSettingsDomain string

var settingsCmd = &cobra.Command{
	Use:   "settings [settings]",
	Short: "Display and update settings",
	Long: `
cozy-stack settings displays the settings.

It can also take a list of settings to update.

If you give a blank value, the setting will be removed.
`,
	Example: "$ cozy-stack settings --domain cozy.tools:8080 context:beta,public_name:John,to_remove:",
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagSettingsDomain == "" {
			errPrintfln("%s", errSettingsMissingDomain)
			return cmd.Usage()
		}
		c := newClient(flagSettingsDomain, consts.Settings)
		res, err := c.Req(&request.Options{
			Method: "GET",
			Path:   "/settings/instance",
		})
		if err != nil {
			return err
		}
		defer res.Body.Close()
		var obj map[string]interface{}
		if err = json.NewDecoder(res.Body).Decode(&obj); err != nil {
			return err
		}
		if len(args) > 0 {
			obj, err = updateSettings(c, obj, args[0])
			if err != nil {
				return err
			}
		}
		printSettings(obj)
		return nil
	},
}

func printSettings(obj map[string]interface{}) {
	data, ok := obj["data"].(map[string]interface{})
	if !ok {
		return
	}
	attrs, ok := data["attributes"].(map[string]interface{})
	if !ok {
		return
	}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("- %s: %v\n", k, attrs[k])
	}
}

func updateSettings(c *client.Client, obj map[string]interface{}, args string) (map[string]interface{}, error) {
	data, ok := obj["data"].(map[string]interface{})
	if !ok {
		return nil, errors.New("data not found")
	}
	attrs, ok := data["attributes"].(map[string]interface{})
	if !ok {
		return nil, errors.New("attributes not found")
	}
	for _, arg := range strings.Split(args, ",") {
		parts := strings.SplitN(arg, ":", 2)
		k := parts[0]
		if len(parts) < 2 || parts[1] == "" {
			delete(attrs, k)
		} else {
			attrs[k] = parts[1]
		}
	}
	delete(obj, "links")
	buf, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	body := bytes.NewReader(buf)
	res, err := c.Req(&request.Options{
		Method: "PUT",
		Path:   "/settings/instance",
		Body:   body,
	})
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	err = json.NewDecoder(res.Body).Decode(&obj)
	return obj, err
}

func init() {
	domain := os.Getenv("COZY_DOMAIN")
	if domain == "" && config.IsDevRelease() {
		domain = defaultDevDomain
	}

	settingsCmd.PersistentFlags().StringVar(&flagSettingsDomain, "domain", domain, "specify the domain name of the instance")
	RootCmd.AddCommand(settingsCmd)
}
