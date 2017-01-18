package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// DefaultStorageDir is the default directory name in which data
// is stored relatively to the cozy-stack binary.
const DefaultStorageDir = "storage"

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "cozy-stack",
	Short: "cozy-stack is the main command",
	Long: `Cozy is a platform that brings all your web services in the same private space.
With it, your web apps and your devices can share data easily, providing you
with a new experience. You can install Cozy on your own hardware where no one
profiles you.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return Configure()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Display the usage/help by default
		return cmd.Help()
	},
	// Do not display usage on error
	SilenceUsage: true,
	// We have our own way to display error messages
	SilenceErrors: true,
}

var cfgFile string

func init() {
	binDir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		panic(err)
	}

	flags := RootCmd.PersistentFlags()
	flags.StringVarP(&cfgFile, "config", "c", "", "configuration file (default \"$HOME/.cozy.yaml\")")

	flags.String("host", "localhost", "server host")
	checkNoErr(viper.BindPFlag("host", flags.Lookup("host")))

	flags.IntP("port", "p", 8080, "server port")
	checkNoErr(viper.BindPFlag("port", flags.Lookup("port")))

	flags.String("subdomains", "nested", "how to structure the subdomains for apps (can be nested or flat)")
	checkNoErr(viper.BindPFlag("subdomains", flags.Lookup("subdomains")))

	flags.String("assets", "", "path to the directory with the assets (use the packed assets by default)")
	checkNoErr(viper.BindPFlag("assets", flags.Lookup("assets")))

	flags.String("admin-host", "localhost", "administration server host")
	checkNoErr(viper.BindPFlag("admin.host", flags.Lookup("admin-host")))

	flags.Int("admin-port", 6060, "administration server port")
	checkNoErr(viper.BindPFlag("admin.port", flags.Lookup("admin-port")))

	flags.String("fs-url", fmt.Sprintf("file://localhost%s/%s", binDir, DefaultStorageDir), "filesystem url")
	checkNoErr(viper.BindPFlag("fs.url", flags.Lookup("fs-url")))

	flags.String("couchdb-host", "localhost", "couchdbdb host")
	checkNoErr(viper.BindPFlag("couchdb.host", flags.Lookup("couchdb-host")))

	flags.Int("couchdb-port", 5984, "couchdbdb port")
	checkNoErr(viper.BindPFlag("couchdb.port", flags.Lookup("couchdb-port")))

	flags.String("mail-host", "", "mail smtp host")
	checkNoErr(viper.BindPFlag("mail.host", flags.Lookup("mail-host")))

	flags.Int("mail-port", 465, "mail smtp port")
	checkNoErr(viper.BindPFlag("mail.port", flags.Lookup("mail-port")))

	flags.String("mail-username", "", "mail smtp username")
	checkNoErr(viper.BindPFlag("mail.username", flags.Lookup("mail-username")))

	flags.String("mail-password", "", "mail smtp password")
	checkNoErr(viper.BindPFlag("mail.password", flags.Lookup("mail-password")))

	flags.Bool("mail-disable-tls", false, "disable smtp over tls")
	checkNoErr(viper.BindPFlag("mail.disable_tls", flags.Lookup("mail-disable-tls")))

	flags.String("log-level", "info", "define the log level")
	checkNoErr(viper.BindPFlag("log.level", flags.Lookup("log-level")))
}

// Configure Viper to read the environment and the optional config file
func Configure() error {
	viper.SetEnvPrefix("cozy")
	viper.AutomaticEnv()

	if cfgFile != "" {
		// Read given config file and skip other paths
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName(config.Filename)
		for _, cfgPath := range config.Paths {
			viper.AddConfigPath(cfgPath)
		}
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, isParseErr := err.(viper.ConfigParseError); isParseErr {
			log.Errorf("Failed to read cozy-stack configurations from %s", viper.ConfigFileUsed())
			return err
		}

		if cfgFile != "" {
			return fmt.Errorf("Could not locate config file: %s\n", cfgFile)
		}
	}

	if viper.ConfigFileUsed() != "" {
		log.Debugf("Using config file: %s", viper.ConfigFileUsed())
	}

	if err := config.UseViper(viper.GetViper()); err != nil {
		return err
	}

	return nil
}

func checkNoErr(err error) {
	if err != nil {
		panic(err)
	}
}
