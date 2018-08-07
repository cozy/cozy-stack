package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// docCmdGroup represents the doc command
var docCmdGroup = &cobra.Command{
	Use:   "doc <command>",
	Short: "Print the documentation",
	Long:  "Print the documentation about the usage of cozy-stack in command-line",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Usage()
	},
}

var manDocCmd = &cobra.Command{
	Use:   "man <directory>",
	Short: "Print the manpages of cozy-stack",
	Long:  `Print the manual pages for using cozy-stack in command-line`,
	Example: `$ mkdir -p ~/share/man
$ export MANPATH=~/share/man:$MANPATH
$ cozy-stack doc man ~/share/man
$ man cozy-stack`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return cmd.Usage()
		}
		header := &doc.GenManHeader{
			Title:   "COZY-STACK",
			Section: "1",
		}
		return doc.GenManTree(RootCmd, header, args[0])
	},
}

var markdownDocCmd = &cobra.Command{
	Use:     "markdown <directory>",
	Short:   "Print the documentation of cozy-stack as markdown",
	Example: `$ cozy-stack doc markdown docs/cli`,
	RunE: func(cmd *cobra.Command, args []string) error {
		directory := "./docs/cli/"
		if len(args) == 1 {
			directory = args[0]
		}
		RootCmd.DisableAutoGenTag = true
		return doc.GenMarkdownTree(RootCmd, directory)
	},
}

func init() {
	docCmdGroup.AddCommand(manDocCmd)
	docCmdGroup.AddCommand(markdownDocCmd)
	RootCmd.AddCommand(docCmdGroup)
}
