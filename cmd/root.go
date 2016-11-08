package cmd

import (
	"fmt"
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// ConfigFilename is the default configuration filename that cozy
// search for
const ConfigFilename = "cozy"

// ConfigPaths is the list of directories used to search for a
// configuration file
var ConfigPaths = []string{
	".cozy",
	"$HOME/.cozy",
	"/etc/cozy",
}

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "cozy-stack",
	Short: "cozy-stack is the main command",
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
	pwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	flags := RootCmd.PersistentFlags()
	flags.StringVarP(&cfgFile, "config", "c", "", "configuration file (default \"$HOME/.cozy.yaml\")")

	flags.StringP("mode", "m", config.BuildMode, "server mode: development or production")
	viper.BindPFlag("mode", flags.Lookup("mode"))

	flags.StringP("host", "", "localhost", "server host")
	viper.BindPFlag("host", flags.Lookup("host"))

	flags.IntP("port", "p", 8080, "server port")
	viper.BindPFlag("port", flags.Lookup("port"))

	flags.String("fs-url", fmt.Sprintf("files://localhost%s/storage", pwd), "filesystem url")
	viper.BindPFlag("fs.url", flags.Lookup("fs-url"))

	flags.String("couchdb-host", "localhost", "couchdbdb host")
	viper.BindPFlag("couchdb.host", flags.Lookup("couchdb-host"))

	flags.Int("couchdb-port", 5984, "couchdbdb port")
	viper.BindPFlag("couchdb.port", flags.Lookup("couchdb-port"))

	flags.String("log-level", "info", "define the log level")
	viper.BindPFlag("log.level", flags.Lookup("log-level"))
}

// Configure Viper to read the environment and the optional config file
func Configure() error {
	viper.SetEnvPrefix("cozy")
	viper.AutomaticEnv()

	if cfgFile != "" {
		// Read given config file and skip other paths
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName(ConfigFilename)
		for _, cfgPath := range ConfigPaths {
			viper.AddConfigPath(cfgPath)
		}
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, isParseErr := err.(viper.ConfigParseError); isParseErr {
			fmt.Fprintf(os.Stderr, "Error: While reading cozy-stack configurations from %s\n", viper.ConfigFileUsed())
			return err
		}

		if cfgFile != "" {
			return fmt.Errorf("Unable to locate config file: %s\n", cfgFile)
		}
	}

	if viper.ConfigFileUsed() != "" {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}

	if err := config.UseViper(viper.GetViper()); err != nil {
		return err
	}

	if err := configureLogger(); err != nil {
		return err
	}

	return nil
}

func configureLogger() error {
	loggerCfg := config.GetConfig().Logger

	logLevel, err := log.ParseLevel(loggerCfg.Level)
	if err != nil {
		return err
	}

	log.SetLevel(logLevel)
	return nil
}
