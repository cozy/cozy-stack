package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/howeyc/gopass"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// DefaultStorageDir is the default directory name in which data
// is stored relatively to the cozy-stack binary.
const DefaultStorageDir = "storage"

var cfgFile string
var flagClientUseHTTPS bool

// ErrUsage is returned by the cmd.Usage() method
var ErrUsage = errors.New("Bad usage of command")

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "cozy-stack",
	Short: "cozy-stack is the main command",
	Long: `Cozy is a platform that brings all your web services in the same private space.
With it, your web apps and your devices can share data easily, providing you
with a new experience. You can install Cozy on your own hardware where no one
profiles you.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return config.Setup(cfgFile)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Display the usage/help by default
		return cmd.Usage()
	},
	// Do not display usage on error
	SilenceUsage: true,
	// We have our own way to display error messages
	SilenceErrors: true,
}

func newClient(domain, scope string) *client.Client {
	// For the CLI client, we rely on the admin APIs to generate a CLI token.
	// We may want in the future rely on OAuth to handle the permissions with
	// more granularity.
	c := newAdminClient()
	token, err := c.GetToken(&client.TokenOptions{
		Domain:   domain,
		Subject:  "CLI",
		Audience: permissions.CLIAudience,
		Scope:    []string{scope},
	})
	if err != nil {
		errPrintfln("Could not generate access to domain %s", domain)
		errPrintfln("%s", err)
		os.Exit(1)
	}
	var scheme string
	if flagClientUseHTTPS {
		scheme = "https"
	} else {
		scheme = "http"
	}
	return &client.Client{
		Addr:       config.ServerAddr(),
		Domain:     domain,
		Scheme:     scheme,
		Authorizer: &request.BearerAuthorizer{Token: token},
	}
}

func newAdminClient() *client.Client {
	var pass []byte
	if !config.IsDevRelease() {
		pass = []byte(os.Getenv("COZY_ADMIN_PASSWORD"))
		if len(pass) == 0 {
			var err error
			fmt.Printf("Password:")
			pass, err = gopass.GetPasswdMasked()
			if err != nil {
				panic(err)
			}
		}
	}
	return &client.Client{
		Domain:     config.AdminServerAddr(),
		Scheme:     "http",
		Authorizer: &request.BasicAuthorizer{Password: string(pass)},
	}
}

func init() {
	usageFunc := RootCmd.UsageFunc()

	RootCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		usageFunc(cmd)
		return ErrUsage
	})

	flags := RootCmd.PersistentFlags()
	flags.StringVarP(&cfgFile, "config", "c", "", "configuration file (default \"$HOME/.cozy.yaml\")")

	flags.String("host", "localhost", "server host")
	checkNoErr(viper.BindPFlag("host", flags.Lookup("host")))

	flags.IntP("port", "p", 8080, "server port")
	checkNoErr(viper.BindPFlag("port", flags.Lookup("port")))

	flags.String("admin-host", "localhost", "administration server host")
	checkNoErr(viper.BindPFlag("admin.host", flags.Lookup("admin-host")))

	flags.Int("admin-port", 6060, "administration server port")
	checkNoErr(viper.BindPFlag("admin.port", flags.Lookup("admin-port")))

	flags.BoolVar(&flagClientUseHTTPS, "client-use-https", false, "if set the client will use https to communicate with the server")
}

func checkNoErr(err error) {
	if err != nil {
		panic(err)
	}
}

func errPrintfln(format string, vals ...interface{}) {
	_, err := fmt.Fprintf(os.Stderr, format+"\n", vals...)
	if err != nil {
		panic(err)
	}
}

func errPrintf(format string, vals ...interface{}) {
	_, err := fmt.Fprintf(os.Stderr, format, vals...)
	if err != nil {
		panic(err)
	}
}
