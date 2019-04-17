package cmd

import (
	"fmt"

	"github.com/cozy/cozy-stack/pkg/dispers"
  //"github.com/cozy/cozy-stack/pkg/ml"
	"github.com/spf13/cobra"
)

// ml represents the machine-learning command
var learningCmdGroup = &cobra.Command{
	Use:     "learning <command>",
	Aliases: []string{"learning"},
	Short:   "Launch some machine learning algorithm",
	Long: `cozy-stack learning allows to launch machine learning algorithm on the data of the stack`,
	RunE: func(cmd *cobra.Command, args []string) error {
    d:=dispers.NewDispers()
    fmt.Println(d.SayHello())
		return nil
	},
}

func init() {
	RootCmd.AddCommand(learningCmdGroup)
}
