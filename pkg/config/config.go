package config

import (
	"bytes"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/gomail"
	"github.com/go-redis/redis"
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

// hardcodedRegistry is the default registry used if no configuration is set
// for registries.
var hardcodedRegistry, _ = url.Parse("https://registry.cozy.io/")

const (
	// FlatSubdomains is the value for apps subdomains like https://<user>-<app>.<domain>/
	FlatSubdomains = "flat"
	// NestedSubdomains is the value for apps subdomains like https://<app>.<user>.<domain>/
	NestedSubdomains = "nested"

	// SchemeFile is the URL scheme used to configure a file filesystem.
	SchemeFile = "file"
	// SchemeMem is the URL scheme used to configure an in-memory filesystem.
	SchemeMem = "mem"
	// SchemeSwift is the URL scheme used to configure a swift filesystem.
	SchemeSwift = "swift"
)

// AdminSecretFileName is the name of the file containing the administration
// hashed passphrase.
const AdminSecretFileName = "cozy-admin-passphrase" // #nosec

var config *Config
var log = logger.WithNamespace("config")

// Config contains the configuration values of the application
type Config struct {
	Host       string
	Port       int
	Assets     string
	Doctypes   string
	Subdomains string
	AdminHost  string
	AdminPort  int
	NoReply    string
	Hooks      string

	AutoUpdates AutoUpdates
	Fs          Fs
	CouchDB     CouchDB
	Jobs        Jobs
	Konnectors  Konnectors
	Mail        *gomail.DialerOptions

	Cache                       RedisConfig
	Lock                        RedisConfig
	SessionStorage              RedisConfig
	DownloadStorage             RedisConfig
	KonnectorsOauthStateStorage RedisConfig
	Realtime                    RedisConfig

	Contexts   map[string]interface{}
	Registries map[string][]*url.URL

	DisableCSP bool
}

// Fs contains the configuration values of the file-system
type Fs struct {
	Auth *url.Userinfo
	URL  *url.URL
}

// CouchDB contains the configuration values of the database
type CouchDB struct {
	Auth *url.Userinfo
	URL  *url.URL
}

// Jobs contains the configuration values for the jobs and triggers synchronization
type Jobs struct {
	Workers int
	Redis   RedisConfig
}

// Konnectors contains the configuration values for the konnectors
type Konnectors struct {
	Cmd string
}

// AutoUpdates contains the configuration values for auto updates
type AutoUpdates struct {
	Activated bool
	Schedule  string
}

// RedisConfig contains the configuration values for a redis system
type RedisConfig struct {
	Auth *url.Userinfo
	URL  *url.URL

	opt *redis.Options
	cli *redis.Client
}

// NewRedisConfig creates a redis configuration and its associated client.
func NewRedisConfig(u string) RedisConfig {
	var conf RedisConfig
	if u == "" {
		return conf
	}
	opt, err := redis.ParseURL(u)
	if err != nil {
		log.Errorf("can't parse redis URL(%s), ignoring", u)
		return conf
	}
	parsedURL, user, err := parseURL(u)
	if err != nil {
		log.Errorf("can't parse redis URL(%s), ignoring", u)
		return conf
	}
	conf.Auth = user
	conf.URL = parsedURL
	conf.cli = redis.NewClient(opt)
	conf.opt = opt
	return conf
}

// FsURL returns a copy of the filesystem URL
func FsURL() *url.URL {
	return config.Fs.URL
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
func CouchURL() *url.URL {
	return config.CouchDB.URL
}

// Client returns the redis.Client for a RedisConfig
func (rc *RedisConfig) Client() *redis.Client {
	return rc.cli
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

// Setup Viper to read the environment and the optional config file
func Setup(cfgFile string) (err error) {
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetEnvPrefix("cozy")
	viper.AutomaticEnv()

	if cfgFile == "" {
		for _, ext := range viper.SupportedExts {
			var file string
			file, err = FindConfigFile(Filename + "." + ext)
			if file != "" && err == nil {
				cfgFile = file
				break
			}
		}
	}

	if cfgFile == "" {
		return UseViper(viper.GetViper())
	}

	log.Debugf("Using config file: %s", cfgFile)

	tmpl := template.New(filepath.Base(cfgFile))
	tmpl = tmpl.Option("missingkey=zero")
	tmpl, err = tmpl.ParseFiles(cfgFile)
	if err != nil {
		return fmt.Errorf("Unable to open and parse configuration file template %s: %s", cfgFile, err)
	}

	dest := new(bytes.Buffer)
	ctxt := &struct{ Env map[string]string }{Env: envMap()}
	err = tmpl.ExecuteTemplate(dest, filepath.Base(cfgFile), ctxt)
	if err != nil {
		return fmt.Errorf("Template error for config file %s: %s", cfgFile, err)
	}

	if ext := filepath.Ext(cfgFile); len(ext) > 0 {
		viper.SetConfigType(ext[1:])
	}
	if err := viper.ReadConfig(dest); err != nil {
		if _, isParseErr := err.(viper.ConfigParseError); isParseErr {
			log.Errorf("Failed to read cozy-stack configurations from %s", cfgFile)
			log.Errorf(dest.String())
			return err
		}
	}

	return UseViper(viper.GetViper())
}

func envMap() map[string]string {
	env := make(map[string]string)
	for _, i := range os.Environ() {
		sep := strings.Index(i, "=")
		env[i[0:sep]] = i[sep+1:]
	}
	return env
}

// UseViper sets the configured instance of Config
func UseViper(v *viper.Viper) error {
	fsURL, err := url.Parse(v.GetString("fs.url"))
	if err != nil {
		return err
	}

	couchURL, couchAuth, err := parseURL(v.GetString("couchdb.url"))
	if err != nil {
		return err
	}
	if couchURL.Path == "" {
		couchURL.Path = "/"
	}

	regs, err := makeRegistries(v)
	if err != nil {
		return err
	}

	config = &Config{
		Host:       v.GetString("host"),
		Port:       v.GetInt("port"),
		Subdomains: v.GetString("subdomains"),
		AdminHost:  v.GetString("admin.host"),
		AdminPort:  v.GetInt("admin.port"),
		Assets:     v.GetString("assets"),
		Doctypes:   v.GetString("doctypes"),
		NoReply:    v.GetString("mail.noreply_address"),
		Hooks:      v.GetString("hooks"),
		Fs: Fs{
			URL: fsURL,
		},
		CouchDB: CouchDB{
			Auth: couchAuth,
			URL:  couchURL,
		},
		Jobs: Jobs{
			Workers: v.GetInt("jobs.workers"),
			Redis:   NewRedisConfig(v.GetString("jobs.url")),
		},
		Konnectors: Konnectors{
			Cmd: v.GetString("konnectors.cmd"),
		},
		AutoUpdates: AutoUpdates{
			Activated: v.GetString("auto_updates.schedule") != "",
			Schedule:  v.GetString("auto_updates.schedule"),
		},
		Cache:                       NewRedisConfig(v.GetString("cache.url")),
		Lock:                        NewRedisConfig(v.GetString("lock.url")),
		SessionStorage:              NewRedisConfig(v.GetString("sessions.url")),
		DownloadStorage:             NewRedisConfig(v.GetString("downloads.url")),
		KonnectorsOauthStateStorage: NewRedisConfig(v.GetString("konnectors.oauthstate")),
		Realtime:                    NewRedisConfig(v.GetString("realtime.url")),
		Mail: &gomail.DialerOptions{
			Host:                      v.GetString("mail.host"),
			Port:                      v.GetInt("mail.port"),
			Username:                  v.GetString("mail.username"),
			Password:                  v.GetString("mail.password"),
			DisableTLS:                v.GetBool("mail.disable_tls"),
			SkipCertificateValidation: v.GetBool("mail.skip_certificate_validation"),
		},
		Contexts:   v.GetStringMap("contexts"),
		Registries: regs,
		DisableCSP: v.GetBool("disable_csp"),
	}

	loggerRedis := NewRedisConfig(v.GetString("log.redis"))
	return logger.Init(logger.Options{
		Level:  v.GetString("log.level"),
		Syslog: v.GetBool("log.syslog"),
		Redis:  loggerRedis.Client(),
	})
}

func makeRegistries(v *viper.Viper) (map[string][]*url.URL, error) {
	regs := make(map[string][]*url.URL)

	regsSlice := v.GetStringSlice("registries")
	if len(regsSlice) > 0 {
		urlList := make([]*url.URL, len(regsSlice))
		for i, s := range regsSlice {
			u, err := url.Parse(s)
			if err != nil {
				return nil, err
			}
			urlList[i] = u
		}
		regs["default"] = urlList
	} else {
		for k, v := range v.GetStringMap("registries") {
			list, ok := v.([]interface{})
			if !ok {
				return nil, fmt.Errorf(
					"Bad format in the registries section of the configuration file: "+
						"should be a list of strings, got %#v", v)
			}
			urlList := make([]*url.URL, len(list))
			for i, s := range list {
				u, err := url.Parse(s.(string))
				if err != nil {
					return nil, err
				}
				urlList[i] = u
			}
			regs[k] = urlList
		}
	}

	defaults, ok := regs["default"]
	if !ok {
		defaults = []*url.URL{hardcodedRegistry}
		regs["default"] = defaults
	}
	for ctx, urls := range regs {
		if ctx == "default" {
			continue
		}
		regs[ctx] = append(urls, defaults...)
	}

	return regs, nil
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

cache:
    url: redis://localhost:6379/0

log:
    level: info

jobs:
    workers: 2
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

func parseURL(u string) (*url.URL, *url.Userinfo, error) {
	parsedURL, err := url.Parse(u)
	if err != nil {
		return nil, nil, err
	}
	user := parsedURL.User
	parsedURL.User = nil
	return parsedURL, user, nil
}
