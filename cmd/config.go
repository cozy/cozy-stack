package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/cozy/cozy-stack/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Display the configuration",
	Long: `Read the environment variables, the config file and
the given parameters to display the configuration.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := json.MarshalIndent(config.GetConfig(), "", "  ")
		if err != nil {
			return err
		}

		fmt.Println(string(cfg))
		return nil
	},
}

func init() {
	RootCmd.AddCommand(configCmd)
}
