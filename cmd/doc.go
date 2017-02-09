package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// docCmdGroup represents the doc command
var docCmdGroup = &cobra.Command{
	Use:   "doc [command]",
	Short: "Print the documentation",
	Long:  "Print the documentation about the usage of cozy-stack in command-line",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}
var manDocCmd = &cobra.Command{
	Use:   "man [directory]",
	Short: "Print the manpages of cozy-stack",
	Long:  `Print the manual pages for using cozy-stack in command-line`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return cmd.Help()
		}
		header := &doc.GenManHeader{
			Title:   "COZY-STACK",
			Section: "1",
		}
		return doc.GenManTree(RootCmd, header, args[0])
	},
}

func init() {
	docCmdGroup.AddCommand(manDocCmd)
	RootCmd.AddCommand(docCmdGroup)
}
