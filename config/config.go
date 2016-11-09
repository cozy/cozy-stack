package config

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/spf13/viper"
)

var (
	// Version of the release (see build.sh script)
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
	Fs      Fs
	CouchDB CouchDB
	Logger  Logger
}

// FsURL returns a copy of the filesystem URL
func (c *Config) FsURL() *url.URL {
	u, err := url.Parse(c.Fs.URL)
	if err != nil {
		panic(fmt.Errorf("Malformed configuration fs url %s.", c.Fs.URL))
	}
	return u
}

// BuildRelFsURL build a new url from the filesystem URL by adding the
// specified relative path.
func (c *Config) BuildRelFsURL(rel string) *url.URL {
	u := c.FsURL()
	if u.Path == "" {
		u.Path = "/"
	}
	u.Path = path.Join(u.Path, rel)
	return u
}

// BuildAbsFsURL build a new url from the filesystem URL by changing
// the path to the specified absolute one.
func (c *Config) BuildAbsFsURL(abs string) *url.URL {
	u := c.FsURL()
	u.Path = path.Join("/", abs)
	return u
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
	Host string
	Port int
}

// Logger contains the configuration values of the logger system
type Logger struct {
	Level string
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

	config = &Config{
		Mode: mode,
		Host: v.GetString("host"),
		Port: v.GetInt("port"),
		Fs: Fs{
			URL: fsURL,
		},
		CouchDB: CouchDB{
			Host: v.GetString("couchdb.host"),
			Port: v.GetInt("couchdb.port"),
		},
		Logger: Logger{
			Level: v.GetString("log.level"),
		},
	}

	return nil
}

// UseTestFile can be used in a test file to inject a configuration
// from a cozy.test.* file. It should receive the relative path to the
// root directory of the repository where the the default
// cozy.test.yaml is installed.
func UseTestFile(rel string) {
	v := viper.New()
	v.SetConfigName("cozy.test")
	v.AddConfigPath(path.Join(rel, ".cozy"))
	v.AddConfigPath("$HOME/.cozy")
	v.AddConfigPath(rel)

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
