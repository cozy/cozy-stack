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
	Short:   "Launch some machine learning algorithms",
	Long: `cozy-stack learning allows to launch machine learning algorithm on the data of the stack`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Usage()
	},
}

// each server in Dispers process has its CmdGroup
var conceptIndexorCmdGroup = &cobra.Command{
	Use:     "conceptindexor <command>",
	Aliases: []string{"conceptindexor"},
	Short:   "Concept Indexor API : pick up data and preprocess them",
	Long: `Concept Indexor API is used in the DISPERS' process to collect the hash of concepts. From this API, you can send a concept and then pick up this concept's hash`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

var dataCmdGroup = &cobra.Command{
	Use:     "data <command>",
	Aliases: []string{"data"},
	Short:   "Data API : pick up data and preprocess them",
	Long: `DATA API is used in the DISPERS' process to train ML models. From this API, you can pick up data, preprocess it. You can also use this API to know on which Data models can be trained`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

var dataAggregatorCmdGroup = &cobra.Command{
	Use:     "dataaggregator <command>",
	Aliases: []string{"dataaggregator"},
	Short:   "Data Aggregator API : aggregate data and train ML models on it.",
	Long: `Data Aggregator API is used in the DISPERS' process to train ML model.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

var targetfinderCmdGroup = &cobra.Command{
	Use:     "trainfinder <command>",
	Aliases: []string{"targetfinder"},
	Short:   "From several lists of adresses, collect the final list of adresses.",
	Long: `Targetfinder API is used in the DISPERS' process to know which adresses are concerned.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

var trainCmdGroup = &cobra.Command{
	Use:     "train <command>",
	Aliases: []string{"train"},
	Short:   "Train some machine learning algorithm",
	Long: `cozy-stack learning train allows to launch the training of a machine learning algorithm on the data of the stack. The process will use some trusted API.`,
	RunE: func(cmd *cobra.Command, args []string) error {
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
	dispersCmdGroup.AddCommand(conceptIndexorCmdGroup)
	dispersCmdGroup.AddCommand(dataCmdGroup)
	dispersCmdGroup.AddCommand(dataAggregatorCmdGroup)
	dispersCmdGroup.AddCommand(targetfinderCmdGroup)
	dispersCmdGroup.AddCommand(trainCmdGroup)
	RootCmd.AddCommand(dispersCmdGroup)
}
