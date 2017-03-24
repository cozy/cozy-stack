package config

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/gomail"
	"github.com/spf13/viper"
)

const (
	// Production mode
	Production string = "production"
	// Development mode
	Development string = "development"
)

var (
	// Version of the release (see scripts/build.sh script)
	Version string
	// BuildTime is ISO-8601 UTC string representation of the time of
	// the build
	BuildTime string
	// BuildMode is the build mode of the release. Should be either
	// production or development.
	BuildMode = Development
)

// Filename is the default configuration filename that cozy
// search for
const Filename = "cozy"

// Paths is the list of directories used to search for a
// configuration file
var Paths = []string{
	".cozy",
	"$HOME/.cozy",
	"/etc/cozy",
}

const (
	// FlatSubdomains is the value for apps subdomains like https://<user>-<app>.<domain>/
	FlatSubdomains = "flat"
	// NestedSubdomains is the value for apps subdomains like https://<app>.<user>.<domain>/
	NestedSubdomains = "nested"
)

// AdminSecretFileName is the name of the file containing the administration
// hashed passphrase.
const AdminSecretFileName = "cozy-admin-passphrase" // #nosec

var config *Config

// Config contains the configuration values of the application
type Config struct {
	Host       string
	Port       int
	Assets     string
	Subdomains string
	AdminHost  string
	AdminPort  int
	Fs         Fs
	CouchDB    CouchDB
	Mail       *gomail.DialerOptions
	Logger     Logger
}

// Fs contains the configuration values of the file-system
type Fs struct {
	URL string
}

// CouchDB contains the configuration values of the database
type CouchDB struct {
	URL string
}

// Logger contains the configuration values of the logger system
type Logger struct {
	Level string
}

// FsURL returns a copy of the filesystem URL
func FsURL() *url.URL {
	u, err := url.Parse(config.Fs.URL)
	if err != nil {
		panic(fmt.Errorf("malformed configuration fs url %s", config.Fs.URL))
	}
	return u
}

// ServerAddr returns the address on which the stack is run
func ServerAddr() string {
	return net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
}

// AdminServerAddr returns the address on which the administration is listening
func AdminServerAddr() string {
	return net.JoinHostPort(config.AdminHost, strconv.Itoa(config.AdminPort))
}

// CouchURL returns the CouchDB string url
func CouchURL() string {
	return config.CouchDB.URL
}

// IsDevRelease returns whether or not the binary is a development
// release
func IsDevRelease() bool {
	return BuildMode == Development
}

// GetConfig returns the configured instance of Config
func GetConfig() *Config {
	return config
}

// UseViper sets the configured instance of Config
func UseViper(v *viper.Viper) error {
	fsURL, err := url.Parse(v.GetString("fs.url"))
	if err != nil {
		return err
	}

	couchURL, err := url.Parse(v.GetString("couchdb.url"))
	if err != nil {
		return err
	}
	if couchURL.Path == "" {
		couchURL.Path = "/"
	}

	config = &Config{
		Host:       v.GetString("host"),
		Port:       v.GetInt("port"),
		Subdomains: v.GetString("subdomains"),
		AdminHost:  v.GetString("admin.host"),
		AdminPort:  v.GetInt("admin.port"),
		Assets:     v.GetString("assets"),
		Fs: Fs{
			URL: fsURL.String(),
		},
		CouchDB: CouchDB{
			URL: couchURL.String(),
		},
		Mail: &gomail.DialerOptions{
			Host:                      v.GetString("mail.host"),
			Port:                      v.GetInt("mail.port"),
			Username:                  v.GetString("mail.username"),
			Password:                  v.GetString("mail.password"),
			DisableTLS:                v.GetBool("mail.disable_tls"),
			SkipCertificateValidation: v.GetBool("mail.skip_certificate_validation"),
		},
		Logger: Logger{
			Level: v.GetString("log.level"),
		},
	}

	return configureLogger()
}

const defaultTestConfig = `
host: localhost
port: 8080
assets: ./assets
subdomains: nested

fs:
  url: mem://test

couchdb:
    url: http://localhost:5984/

log:
    level: info
`

// UseTestFile can be used in a test file to inject a configuration
// from a cozy.test.* file. If it can not find this file in your
// $HOME/.cozy directory it will use the default one.
func UseTestFile() {
	v := viper.New()
	v.SetConfigName("cozy.test")
	v.AddConfigPath("$HOME/.cozy")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			panic(fmt.Errorf("fatal error test config file: %s", err))
		}
		UseTestYAML(defaultTestConfig)
		return
	}

	if err := UseViper(v); err != nil {
		panic(fmt.Errorf("fatal error test config file: %s", err))
	}

	return
}

// UseTestYAML can be used in a test file to inject a configuration
// from a YAML string.
func UseTestYAML(yaml string) {
	v := viper.New()
	v.SetConfigType("yaml")

	if err := v.ReadConfig(strings.NewReader(yaml)); err != nil {
		panic(fmt.Errorf("fatal error test config file: %s", err))
	}

	if err := UseViper(v); err != nil {
		panic(fmt.Errorf("fatal error test config file: %s", err))
	}

	return
}

// FindConfigFile search in the Paths directories for the file with the given
// name. It returns an error if it cannot find it or if an error occurs while
// searching.
func FindConfigFile(name string) (string, error) {
	for _, cp := range Paths {
		filename := filepath.Join(utils.AbsPath(cp), name)
		ok, err := utils.FileExists(filename)
		if err != nil {
			return "", err
		}
		if ok {
			return filename, nil
		}
	}
	return "", fmt.Errorf("Could not find config file %s", name)
}

func configureLogger() error {
	loggerCfg := config.Logger

	level := loggerCfg.Level
	if level == "" {
		level = "info"
	}

	logLevel, err := log.ParseLevel(level)
	if err != nil {
		return err
	}

	log.SetLevel(logLevel)
	return nil
}
