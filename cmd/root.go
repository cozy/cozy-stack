package cmd

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
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

const defaultDevDomain = "cozy.tools:8080"

var cfgFile string

// ErrUsage is returned by the cmd.Usage() method
var ErrUsage = errors.New("Bad usage of command")

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

func sslVerifyPinnedKey(fingerprint string) (func(certs [][]byte, verifiedChains [][]*x509.Certificate) error, error) {
	pinnedFingerPrint, err := hex.DecodeString(fingerprint)
	if err != nil {
		return nil, fmt.Errorf("Invalid fingerprint encoding for %s", fingerprint)
	}

	expected := sha256.Size
	given := len(pinnedFingerPrint)
	if given != expected {
		return nil, fmt.Errorf("Invalid fingerprint size for %s, expected %d got %d", fingerprint,
			expected, given)
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
	}, nil
}

func sslClient(e *endpoint) (*http.Client, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: !e.Validate,
	}

	ca := e.CA
	if ca != "" {
		data, err := ioutil.ReadFile(ca)
		if err != nil {
			return nil, fmt.Errorf("Could not read file %q: %s", ca, err)
		}
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(data)
		tlsConfig.RootCAs = pool
	}

	cert := e.Cert
	key := e.Key
	if cert != "" && key != "" {
		pair, err := tls.LoadX509KeyPair(cert, key)
		if err != nil {
			return nil, fmt.Errorf("Could not read client certificate files %q and %q: %s",
				cert, key, err)
		}
		tlsConfig.Certificates = []tls.Certificate{pair}
	}

	fp := e.Fingerprint
	if fp != "" {
		check, err := sslVerifyPinnedKey(fp)
		if err != nil {
			return nil, err
		}
		tlsConfig.VerifyPeerCertificate = check
	}

	return &http.Client{
		Timeout:   e.Timeout,
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
	}, nil
}

type endpoint struct {
	URL         *url.URL
	Cert        string
	Key         string
	CA          string
	Fingerprint string
	Validate    bool
	Timeout     time.Duration
}

func (e *endpoint) generateURL(host string, port int) error {
	u, err := url.Parse(host)
	if err != nil {
		return err
	}

	if u.Scheme == "" {
		// We have host + port, HTTP implied
		u = &url.URL{
			Scheme: "http",
			Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		}
	}

	e.URL = u

	return nil
}

func (e *endpoint) configureFromURL() error {
	u := e.URL
	query := u.Query()

	if t := query.Get("timeout"); t != "" {
		timeout, err := time.ParseDuration(t)
		if err != nil {
			return err
		}
		e.Timeout = timeout
	}

	if u.Scheme == "https" {
		if t := query.Get("ca"); t != "" {
			e.CA = t
		}
		if t := query.Get("cert"); t != "" {
			e.Cert = t
		}
		if t := query.Get("key"); t != "" {
			e.Key = t
		}
		if t := query.Get("fp"); t != "" {
			e.Fingerprint = t
		}
		if t := query.Get("validate"); t != "" {
			validate, err := strconv.ParseBool(t)
			if err != nil {
				return err
			}
			e.Validate = validate
		}
	}

	return nil
}

func (e *endpoint) configureFromEnv(prefix string) error {
	if t := os.Getenv(prefix + "_CERT"); t != "" {
		e.Cert = t
	}
	if t := os.Getenv(prefix + "_KEY"); t != "" {
		e.Key = t
	}
	if t := os.Getenv(prefix + "_CA"); t != "" {
		e.CA = t
	}
	if t := os.Getenv(prefix + "_FINGERPRINT"); t != "" {
		e.Fingerprint = t
	}

	if t := os.Getenv(prefix + "_VALIDATE"); t != "" {
		validate, err := strconv.ParseBool(t)
		if err != nil {
			return err
		}
		e.Validate = validate
	}

	if t := os.Getenv(prefix + "_TIMEOUT"); t != "" {
		timeout, err := time.ParseDuration(t)
		if err != nil {
			return err
		}
		e.Timeout = timeout
	}

	return nil
}

func (e *endpoint) configure(prefix string, host string, port int) error {
	e.Validate = true
	e.Timeout = 5 * time.Minute

	if err := e.generateURL(host, port); err != nil {
		return err
	}
	if err := e.configureFromEnv(prefix); err != nil {
		return err
	}
	return e.configureFromURL()
}

func (e *endpoint) getClient() (*http.Client, error) {
	u := e.URL
	if u.Scheme == "https" {
		return sslClient(e)
	}
	return &http.Client{
		Timeout: e.Timeout,
	}, nil
}

func newClientSafe(domain string, scopes ...string) (*client.Client, error) {
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
		return nil, err
	}

	cfg := config.GetConfig()
	e := endpoint{}
	err = e.configure("COZY_HOST", cfg.Host, cfg.Port)
	if err != nil {
		return nil, err
	}

	h, err := e.getClient()
	if err != nil {
		return nil, err
	}

	u := e.URL

	return &client.Client{
		Scheme:     u.Scheme,
		Addr:       u.Host,
		Domain:     domain,
		Client:     h,
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
	e := endpoint{}

	err := e.configure("COZY_ADMIN", cfg.AdminHost, cfg.AdminPort)
	checkNoErr(err)

	h, err := e.getClient()
	checkNoErr(err)

	u := e.URL
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
