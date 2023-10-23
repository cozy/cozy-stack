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
commands.

Bash:

  $ source <(cozy-stack completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ cozy-stack completion bash > /etc/bash_completion.d/cozy-stack
  # macOS:
  $ cozy-stack completion bash > $(brew --prefix)/etc/bash_completion.d/cozy-stack

Note: this requires the bash-completion framework, which is not installed by
default on Mac.  This can be installed by using homebrew:

    $ brew install bash-completion

Once installed, bash_completion must be evaluated.  This can be done by adding the
following line to the .bash_profile

    $ source $(brew --prefix)/etc/bash_completion

Zsh:

  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:

  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ cozy-stack completion zsh > "${fpath[1]}/_cozy-stack"

  # You will need to start a new shell for this setup to take effect.

fish:

  $ cozy-stack completion fish | source

  # To load completions for each session, execute once:
  $ cozy-stack completion fish > /etc/fish/completions/cozy-stack.fish
`,
	ValidArgs: []string{"bash", "zsh", "fish"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return cmd.Usage()
		}
		switch args[0] {
		case "bash":
			return RootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
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
