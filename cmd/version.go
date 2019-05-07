package cmd

import (
	"fmt"

	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Long:  `Print the current version number of the binary`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(build.Version)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(versionCmd)
}
