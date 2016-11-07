package config

import (
	"fmt"

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
	CouchDB CouchDB
	Logger  Logger
}

const (
	// Production mode
	Production string = "production"
	// Development mode
	Development string = "development"
)

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
func UseViper(viper *viper.Viper) error {
	mode, err := parseMode(viper.GetString("mode"))
	if err != nil {
		return err
	}

	config = &Config{
		Mode: mode,
		Host: viper.GetString("host"),
		Port: viper.GetInt("port"),
		CouchDB: CouchDB{
			Host: viper.GetString("couchdb.host"),
			Port: viper.GetInt("couchdb.port"),
		},
		Logger: Logger{
			Level: viper.GetString("log.level"),
		},
	}

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
