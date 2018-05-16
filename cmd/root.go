package cmd

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/howeyc/gopass"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

// DefaultStorageDir is the default directory name in which data
// is stored relatively to the cozy-stack binary.
const DefaultStorageDir = "storage"

var cfgFile string

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

func sslVerifyPinnedKey(pinnedFingerPrint []byte) func(certs [][]byte, verifiedChains [][]*x509.Certificate) error {
	if len(pinnedFingerPrint) != sha256.Size {
		panic("key len should be 32")
	}

	return func(certs [][]byte, verifiedChains [][]*x509.Certificate) error {
		// Check for leaf pinning first
		for _, asn1 := range certs {
			cert, err := x509.ParseCertificate(asn1)
			if err != nil {
				return err
			}
			fingerPrint := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
			if bytes.Equal(pinnedFingerPrint, fingerPrint[:]) {
				return nil
			}
		}

		// Then check for intermediate pinning
		for _, verifiedChain := range verifiedChains {
			if len(verifiedChain) > 0 {
				verifiedCert := verifiedChain[0]
				fingerPrint := sha256.Sum256(verifiedCert.RawSubjectPublicKeyInfo)
				if bytes.Equal(pinnedFingerPrint, fingerPrint[:]) {
					return nil
				}
			}
		}
		return fmt.Errorf("ssl: could not find the valid pinned key from proposed ones")
	}
}

func sslClient(ca string, cert string, key string, fp string, verify bool, timeout time.Duration) (*http.Client, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: !verify,
	}

	if ca != "" {
		data, err := ioutil.ReadFile(ca)
		if err != nil {
			return nil, fmt.Errorf("Could not read file %q: %s", ca, err)
		}
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(data)
		tlsConfig.RootCAs = pool
	}

	if cert != "" && key != "" {
		cert, err := tls.LoadX509KeyPair(cert, key)
		if err != nil {
			return nil, fmt.Errorf("Could not read client certificate files %q and %q: %s",
				cert, key, err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if fp != "" {
		pinnedFingerPrint, err := hex.DecodeString(fp)
		if err != nil {
			return nil, fmt.Errorf("Invalid fingerprint encoding for %s", fp)
		}
		if len(pinnedFingerPrint) != sha256.Size {
			return nil, fmt.Errorf("Invalid fingerprint size for %s, expected %d got %d", fp,
				sha256.Size, len(pinnedFingerPrint))
		}
		tlsConfig.VerifyPeerCertificate = sslVerifyPinnedKey(pinnedFingerPrint)
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
	}, nil
}

func configureEndpoint(u *url.URL) (result *url.URL, client *http.Client, err error) {
	if u.Scheme == "https" {
		query := u.Query()
		ca := query.Get("ca")
		cert := query.Get("cert")
		key := query.Get("key")
		fp := query.Get("fp")
		v := query.Get("validate")

		var validate bool
		if v == "" {
			validate = true
		} else {
			validate, err = strconv.ParseBool(v)
			if err != nil {
				return nil, nil, err
			}
		}

		timeout := 0 * time.Second
		t := query.Get("timeout")
		if t != "" {
			timeout, err = time.ParseDuration(t)
			if err != nil {
				return nil, nil, err
			}
		}

		client, err = sslClient(ca, cert, key, fp, validate, timeout)
		if err != nil {
			return nil, nil, err
		}
	}

	// Remove others parts
	result = &url.URL{
		User:   u.User,
		Scheme: u.Scheme,
		Host:   u.Host,
	}

	return result, client, nil
}

func parseEndpoint(host string, port int) (*url.URL, *http.Client, error) {
	u, err := url.Parse(host)
	if err != nil {
		return nil, nil, err
	}

	if u.Scheme == "" {
		// We have host + port, HTTP implied
		u = &url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		}
	}

	url, client, err := configureEndpoint(u)
	if err != nil {
		return nil, nil, err
	}
	return url, client, nil
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

	cfg := config.GetConfig()
	u, h, err := parseEndpoint(cfg.Host, cfg.Port)
	checkNoErr(err)

	return &client.Client{
		Scheme:     u.Scheme,
		Addr:       u.Host,
		Domain:     domain,
		Client:     h,
		Authorizer: &request.BearerAuthorizer{Token: token},
	}
}

func newAdminClient() *client.Client {
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

	cfg := config.GetConfig()
	u, h, err := parseEndpoint(cfg.AdminHost, cfg.AdminPort)
	checkNoErr(err)

	c := &client.Client{
		Scheme:     u.Scheme,
		Addr:       u.Host,
		Domain:     u.Host,
		Client:     h,
		Authorizer: &request.BasicAuthorizer{Password: string(pass)},
	}

	return c
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
