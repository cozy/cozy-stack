package config

import (
	"github.com/spf13/viper"
)

var config *Config

// Config contains the configuration values of the application
type Config struct {
	Mode    Mode
	Host    string
	Port    int
	CouchDB CouchDB
	Logger  Logger
}

// Mode is how is started the server, eg. production or development
type Mode string

const (
	// Production mode
	Production Mode = "production"
	// Development mode
	Development Mode = "development"
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
	config = &Config{
		Mode: parseMode(viper.GetString("mode")),
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

func parseMode(mode string) Mode {
	if mode == "production" {
		return Production
	}

	return Development
}
