package config

import (
	"fmt"
	"net/url"
	"path"
	"runtime"
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/viper"
)

var (
	// Version of the release (see scripts/build.sh script)
	Version string
	// BuildTime is ISO-8601 UTC string representation of the time of
	// the build
	BuildTime string
	// BuildMode is the build mode of the release. Should be either
	// production or development.
	BuildMode = "development"
)

var config *Config

// Config contains the configuration values of the application
type Config struct {
	Mode    string
	Host    string
	Port    int
	Assets  string
	Fs      Fs
	CouchDB CouchDB
	Logger  Logger
}

const (
	// Production mode
	Production string = "production"
	// Development mode
	Development string = "development"
)

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
		panic(fmt.Errorf("Malformed configuration fs url %s.", config.Fs.URL))
	}
	return u
}

// BuildRelFsURL build a new url from the filesystem URL by adding the
// specified relative path.
func BuildRelFsURL(rel string) *url.URL {
	u := FsURL()
	if u.Path == "" {
		u.Path = "/"
	}
	u.Path = path.Join(u.Path, rel)
	return u
}

// BuildAbsFsURL build a new url from the filesystem URL by changing
// the path to the specified absolute one.
func BuildAbsFsURL(abs string) *url.URL {
	u := FsURL()
	u.Path = path.Join("/", abs)
	return u
}

// ServerAddr returns the address on which the stack is run
func ServerAddr() string {
	return config.Host + ":" + strconv.Itoa(config.Port)
}

// CouchURL returns the CouchDB string url
func CouchURL() string {
	return config.CouchDB.URL
}

// IsMode returns whether or not the mode is equal to the specified
// one
func IsMode(mode string) bool {
	return config.Mode == mode
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
	mode, err := parseMode(v.GetString("mode"))
	if err != nil {
		return err
	}

	fsURL := v.GetString("fs.url")
	_, err = url.Parse(fsURL)
	if err != nil {
		return err
	}

	couchHost := v.GetString("couchdb.host")
	couchPort := strconv.Itoa(v.GetInt("couchdb.port"))
	couchURL := "http://" + couchHost + ":" + couchPort + "/"
	_, err = url.Parse(couchURL)
	if err != nil {
		return err
	}

	domain := v.GetString("domain")
	if domain == "" && IsDevRelease() {
		domain = "localhost"
	}

	if domain == "" {
		return fmt.Errorf("missing domain name")
	}

	config = &Config{
		Mode:   mode,
		Host:   v.GetString("host"),
		Port:   v.GetInt("port"),
		Assets: v.GetString("assets"),
		Fs: Fs{
			URL: fsURL,
		},
		CouchDB: CouchDB{
			URL: couchURL,
		},
		Logger: Logger{
			Level: v.GetString("log.level"),
		},
	}

	return configureLogger()
}

// UseTestFile can be used in a test file to inject a configuration
// from a cozy.test.* file. It should receive the relative path to the
// root directory of the repository where the the default
// cozy.test.yaml is installed.
func UseTestFile() {
	_, repo, _, _ := runtime.Caller(0)
	repo = path.Join(repo, "../..")

	v := viper.New()
	v.SetConfigName("cozy.test")
	v.AddConfigPath(path.Join(repo, ".cozy"))
	v.AddConfigPath("$HOME/.cozy")
	v.AddConfigPath(repo)

	if err := v.ReadInConfig(); err != nil {
		panic(fmt.Errorf("Fatal error test config file: %s\n", err))
	}

	if err := UseViper(v); err != nil {
		panic(fmt.Errorf("Fatal error test config file: %s\n", err))
	}

	return
}

// UseTestYAML can be used in a test file to inject a configuration
// from a YAML string.
func UseTestYAML(yaml string) {
	v := viper.New()

	if err := v.ReadConfig(strings.NewReader(yaml)); err != nil {
		panic(fmt.Errorf("Fatal error test config file: %s\n", err))
	}

	if err := UseViper(v); err != nil {
		panic(fmt.Errorf("Fatal error test config file: %s\n", err))
	}

	return
}

func configureLogger() error {
	loggerCfg := config.Logger

	logLevel, err := log.ParseLevel(loggerCfg.Level)
	if err != nil {
		return err
	}

	log.SetLevel(logLevel)
	return nil
}

func parseMode(mode string) (string, error) {
	if BuildMode == Production && mode != Production {
		return "", fmt.Errorf("Only production mode is allowed in this version")
	}

	if BuildMode == Development && mode == "" {
		mode = Development
	}

	if mode == Production {
		return Production, nil
	}

	if mode == Development {
		return Development, nil
	}

	return "", fmt.Errorf("Unknown mode %s", mode)
}
