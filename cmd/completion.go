package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion <shell>",
	Short: "Output shell completion code for the specified shell",
	Long: `
Output shell completion code for the specified shell (bash, zsh, or fish). The
shell code must be evalutated to provide interactive completion of cozy-stack
commands.  This can be done by sourcing it from the .bash_profile.

Note: this requires the bash-completion framework, which is not installed by
default on Mac.  This can be installed by using homebrew:

    $ brew install bash-completion

Once installed, bash_completion must be evaluated.  This can be done by adding the
following line to the .bash_profile

    $ source $(brew --prefix)/etc/bash_completion`,
	Example:   `# cozy-stack completion bash > /etc/bash_completion.d/cozy-stack`,
	ValidArgs: []string{"bash", "zsh", "fish"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return cmd.Usage()
		}
		switch args[0] {
		case "bash":
			return RootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			// Zsh completion support is still basic
			// https://github.com/spf13/cobra/issues/107
			return RootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			includeDescription := true
			return RootCmd.GenFishCompletion(os.Stdout, includeDescription)
		}
		return errors.New("Unsupported shell")
	},
}

func init() {
	RootCmd.AddCommand(completionCmd)
}
