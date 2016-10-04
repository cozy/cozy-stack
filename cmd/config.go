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
		if err := Configure(); err != nil {
			return err
		}

		cfg, err := json.MarshalIndent(config.GetConfig(), "", "  ")
		fmt.Println(string(cfg))
		return err
	},
}

func init() {
	RootCmd.AddCommand(configCmd)
}
