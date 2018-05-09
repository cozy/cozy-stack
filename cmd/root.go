package cmd

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

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

func newClient(domain string, scopes ...string) *client.Client {
	// For the CLI client, we rely on the admin APIs to generate a CLI token.
	// We may want in the future rely on OAuth to handle the permissions with
	// more granularity.
	c := newAdminClient()
	token, err := c.GetToken(&client.TokenOptions{
		Domain:   domain,
		Subject:  "CLI",
		Audience: permissions.CLIAudience,
		Scope:    scopes,
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
	var err error
	useHTTPS := false
	if envHTTPS := os.Getenv("COZY_ADMIN_HTTPS_CLIENT"); envHTTPS != "" {
		useHTTPS, err = strconv.ParseBool(envHTTPS)
		if err != nil {
			errFatalf("Could not read COZY_ADMIN_HTTPS variable: %s", err)
		}
	}
	pass := []byte(os.Getenv("COZY_ADMIN_PASSWORD"))
	if !config.IsDevRelease() {
		if len(pass) == 0 {
			var err error
			fmt.Printf("Password:")
			pass, err = gopass.GetPasswdMasked()
			if err != nil {
				errFatalf("Could not get password from standard input: %s\n", err)
			}
		}
	}
	c := &client.Client{
		Domain:     config.AdminServerAddr(),
		Authorizer: &request.BasicAuthorizer{Password: string(pass)},
	}
	if useHTTPS {
		c.Scheme = "https"
		c.Client = sslClient()
	} else {
		c.Scheme = "http"
	}
	return c
}

func sslClient() *http.Client {
	var rootCAs *x509.CertPool
	var clientCertificate tls.Certificate
	var verifyPeerCertificate func(_ [][]byte, verifiedChains [][]*x509.Certificate) error

	if envRootCA := os.Getenv("COZY_ADMIN_HTTPS_CLIENT_ROOTCA_FILE"); envRootCA != "" {
		rootCA, err := ioutil.ReadFile(envRootCA)
		if err != nil {
			errFatalf("Could not read file %q: %s", envRootCA, err)
		}
		rootCAs = x509.NewCertPool()
		rootCAs.AppendCertsFromPEM(rootCA)
	}

	if envClientCert := os.Getenv("COZY_ADMIN_HTTPS_CLIENT_CERT_FILE"); envClientCert != "" {
		envClientKeyFile := os.Getenv("COZY_ADMIN_HTTPS_CLIENT_KEY_FILE")
		cert, err := tls.LoadX509KeyPair(envClientCert, envClientKeyFile)
		if err != nil {
			errFatalf("Could not read client certificate files %q and %q: %s",
				envClientCert, envClientKeyFile, err)
		}
		clientCertificate = cert
	}

	if envKeyPinned := os.Getenv("COZY_ADMIN_HTTPS_CLIENT_KEYPINNED_FINGERPRINT"); envKeyPinned != "" {
		pinnedFingerPrint, err := base64.StdEncoding.DecodeString(envKeyPinned)
		if err != nil {
			errFatalf("Invalid encoding for COZY_ADMIN_HTTPS_CLIENT_KEYPINNED_FINGERPRINT")
		}
		if len(pinnedFingerPrint) != sha256.Size {
			errFatalf("Invalid size for COZY_ADMIN_HTTPS_CLIENT_KEYPINNED: expected %d got %d",
				sha256.Size, len(pinnedFingerPrint))
		}
		verifyPeerCertificate = sslVerifyPinnedKey(pinnedFingerPrint)
	}

	tlsConfig := &tls.Config{
		Certificates:          []tls.Certificate{clientCertificate},
		RootCAs:               rootCAs,
		VerifyPeerCertificate: verifyPeerCertificate,
		InsecureSkipVerify:    false, // should be false, we *need* rootca verification
	}
	return &http.Client{
		Timeout:   15 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
	}
}

func sslVerifyPinnedKey(pinnedFingerPrint []byte) func(_ [][]byte, verifiedChains [][]*x509.Certificate) error {
	if len(pinnedFingerPrint) != sha256.Size {
		panic("key len should be 32")
	}
	return func(_ [][]byte, verifiedChains [][]*x509.Certificate) error {
		// InsecureSkipVerify is not activated, the chain has been verified when we
		// enter this callback. It should never be empty. This is an extra-check.
		// For more infos: https://golang.org/pkg/crypto/tls/#Config
		if len(verifiedChains) == 0 || len(verifiedChains[0]) == 0 {
			return fmt.Errorf("ssl: certificate verified chains is empty")
		}
		verifiedCert := verifiedChains[0][0]
		fingerPrint := sha256.Sum256(verifiedCert.RawSubjectPublicKeyInfo)
		if !bytes.Equal(pinnedFingerPrint, fingerPrint[:]) {
			return fmt.Errorf("ssl: could not find the valid pinned key from proposed ones")
		}
		return nil
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

func errFatalf(format string, vals ...interface{}) {
	_, err := fmt.Fprintf(os.Stderr, format, vals...)
	if err != nil {
		panic(err)
	}
	os.Exit(1)
}
