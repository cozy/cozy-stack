package cmd

import (
	"fmt"

	"github.com/cozy/cozy-stack/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "cozy",
	Short: "cozy is the main command",
	Long: `Cozy is a platform that brings all your web services in the same private space.
With it, your web apps and your devices can share data easily, providing you
with a new experience. You can install Cozy on your own hardware where no one
profiles you.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := Configure(); err != nil {
			return err
		}

		// Display the usage/help by default
		return cmd.Help()
	},
	// Do not display usage on error
	SilenceUsage: true,
}

var cfgFile string

func init() {
	RootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is $HOME/.cozy.yaml)")

	RootCmd.PersistentFlags().StringP("mode", "m", "development", "server mode: development or production")
	viper.BindPFlag("mode", RootCmd.PersistentFlags().Lookup("mode"))

	RootCmd.PersistentFlags().StringP("address", "a", "localhost", "Address on which the server will listen")
	viper.BindPFlag("address", RootCmd.PersistentFlags().Lookup("address"))

	RootCmd.PersistentFlags().IntP("port", "p", 8080, "Port on which the server will listen")
	viper.BindPFlag("port", RootCmd.PersistentFlags().Lookup("port"))

	RootCmd.PersistentFlags().StringP("databaseUrl", "d", "http://localhost:5984", "Database to connect to")
	viper.BindPFlag("databaseUrl", RootCmd.PersistentFlags().Lookup("databaseUrl"))
}

// Configure Viper to read the environment and the optional config file
func Configure() error {
	viper.SetEnvPrefix("cozy")
	viper.AutomaticEnv()

	if cfgFile != "" {
		// Read given config file and skip other paths
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName(".cozy")
		viper.AddConfigPath("/etc/cozy")
		viper.AddConfigPath("$HOME")
		viper.AddConfigPath(".")
	}

	err := viper.ReadInConfig()

	if err != nil {
		if _, ok := err.(viper.ConfigParseError); ok {
			return err
		}

		if cfgFile != "" {
			return fmt.Errorf("Unable to locate config file: %s\n", cfgFile)
		}
	}

	if viper.ConfigFileUsed() != "" {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}

	config.UseViper(viper.GetViper())

	return nil
}
