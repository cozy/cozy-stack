package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/client/request"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/tlsclient"
	"github.com/howeyc/gopass"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// DefaultStorageDir is the default directory name in which data
// is stored relatively to the cozy-stack binary.
const DefaultStorageDir = "storage"

const defaultDevDomain = "cozy.localhost:8080"

var flagDomain string

var cfgFile string

var errMissingDomain = errors.New("Missing --domain flag, or COZY_DOMAIN env variable")

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "cozy-stack <command>",
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

func newClientSafe(domain string, scopes ...string) (*client.Client, error) {
	// For the CLI client, we rely on the admin APIs to generate a CLI token.
	// We may want in the future rely on OAuth to handle the permissions with
	// more granularity.
	c := newAdminClient()
	token, err := c.GetToken(&client.TokenOptions{
		Domain:   domain,
		Subject:  "CLI",
		Audience: consts.CLIAudience,
		Scope:    scopes,
	})
	if err != nil {
		return nil, err
	}

	httpClient, clientURL, err := tlsclient.NewHTTPClient(tlsclient.HTTPEndpoint{
		Host:      config.GetConfig().Host,
		Port:      config.GetConfig().Port,
		Timeout:   5 * time.Minute,
		EnvPrefix: "COZY_HOST",
	})
	if err != nil {
		return nil, err
	}
	return &client.Client{
		Scheme:     clientURL.Scheme,
		Addr:       clientURL.Host,
		Domain:     domain,
		Client:     httpClient,
		Authorizer: &request.BearerAuthorizer{Token: token},
	}, nil
}

func newClient(domain string, scopes ...string) *client.Client {
	client, err := newClientSafe(domain, scopes...)
	if err != nil {
		errPrintfln("Could not generate access to domain %s", domain)
		errPrintfln("%s", err)
		os.Exit(1)
	}
	return client
}

func newAdminClient() *client.Client {
	pass := []byte(os.Getenv("COZY_ADMIN_PASSWORD"))
	if !build.IsDevRelease() {
		if len(pass) == 0 {
			var err error
			fmt.Printf("Password:")
			pass, err = gopass.GetPasswdMasked()
			if err != nil {
				errFatalf("Could not get password from standard input: %s\n", err)
			}
		}
	}

	httpClient, adminURL, err := tlsclient.NewHTTPClient(tlsclient.HTTPEndpoint{
		Host:      config.GetConfig().AdminHost,
		Port:      config.GetConfig().AdminPort,
		Timeout:   10 * time.Minute,
		EnvPrefix: "COZY_ADMIN",
	})
	checkNoErr(err)

	return &client.Client{
		Scheme:     adminURL.Scheme,
		Addr:       adminURL.Host,
		Domain:     adminURL.Host,
		Client:     httpClient,
		Authorizer: &request.BasicAuthorizer{Password: string(pass)},
	}
}

func init() {
	usageFunc := RootCmd.UsageFunc()
	RootCmd.SetUsageFunc(func(cmd *cobra.Command) error {
		_ = usageFunc(cmd)
		return nil
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

func errFatalf(format string, vals ...interface{}) {
	_, err := fmt.Fprintf(os.Stderr, format, vals...)
	if err != nil {
		panic(err)
	}
	os.Exit(1)
}

func deprecatedDomainArg() {
	errPrintfln("Please use --domain, the positional argument for domain has been deprecated")
}

func cozyDomain() string {
	domain := os.Getenv("COZY_DOMAIN")
	if domain == "" && build.IsDevRelease() {
		domain = defaultDevDomain
	}
	return domain
}

func prompt(text string) string {
	fmt.Fprintf(os.Stderr, "%s ", text)
	r := bufio.NewReader(os.Stdin)
	s, err := r.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
	return strings.TrimSuffix(s, "\n")
}
