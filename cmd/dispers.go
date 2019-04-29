package cmd

import (
	"fmt"

	"github.com/cozy/cozy-stack/pkg/dispers"
	"github.com/spf13/cobra"
)

// ml represents the machine-learning command
var dispersCmdGroup = &cobra.Command{
	Use:     "learning <command>",
	Aliases: []string{"learning"},
	Short:   "Launch some machine learning algorithm",
	Long: `cozy-stack learning allows to launch machine learning algorithm on the data of the stack`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Usage()
	},
}

var trainCmdGroup = &cobra.Command{
	Use:     "train <command>",
	Aliases: []string{"train"},
	Short:   "Train some machine learning algorithm",
	Long: `cozy-stack learning train allows to launch the training of a machine learning algorithm on the data of the stack. The process will use some trusted API.`,
	RunE: func(cmd *cobra.Command, args []string) error {
    fmt.Println("Ici, vous pourrez lancer un traitement ML")
		return nil
	},
}

var dataCmdGroup = &cobra.Command{
	Use:     "data <command>",
	Aliases: []string{"data"},
	Short:   "Data API : pick up data and preprocess them",
	Long: `DATA API is used in the DISPERS' process to train ML models. From this API, you can pick up data, preprocess it. You can also use this API to know on which Data models can be trained`,
	RunE: func(cmd *cobra.Command, args []string) error {
    fmt.Println(dispers.DataSayHello())
		return nil
	},
}

var allData = &cobra.Command{
	Use:     "ls",
	Short:   "Print every supported Data",
	Long: `Know on which Data models can be trained`,
	RunE: func(cmd *cobra.Command, args []string) error {
		for i := 0; i < len(dispers.SupportedData); i++ {
			fmt.Println("[",i,"] ",dispers.SupportedData[i])
		}
		return nil
	},

}


func init() {
	dataCmdGroup.AddCommand(allData)
	dispersCmdGroup.AddCommand(dataCmdGroup)
	dispersCmdGroup.AddCommand(trainCmdGroup)
	RootCmd.AddCommand(dispersCmdGroup)
}
