package config

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
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

// AdminSecretFileName is the name of the file containing the administration
// hashed passphrase.
const AdminSecretFileName = "cozy-admin-passphrase"

var config *Config

// Config contains the configuration values of the application
type Config struct {
	Mode      string
	Host      string
	Port      int
	AdminHost string
	AdminPort int
	Assets    string
	Fs        Fs
	CouchDB   CouchDB
	Logger    Logger
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

// AdminServerAddr returns the address on which the administration is listening
func AdminServerAddr() string {
	return config.AdminHost + ":" + strconv.Itoa(config.AdminPort)
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

	config = &Config{
		Mode:      mode,
		Host:      v.GetString("host"),
		Port:      v.GetInt("port"),
		AdminHost: v.GetString("admin.host"),
		AdminPort: v.GetInt("admin.port"),
		Assets:    v.GetString("assets"),
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

const defaultTestConfig = `
mode: development
host: localhost
port: 8080
assets: ./assets

fs:
  url: mem://test

couchdb:
    host: localhost
    port: 5984

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
			panic(fmt.Errorf("Fatal error test config file: %s\n", err))
		}
		UseTestYAML(defaultTestConfig)
		return
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
	v.SetConfigType("yaml")

	if err := v.ReadConfig(strings.NewReader(yaml)); err != nil {
		panic(fmt.Errorf("Fatal error test config file: %s\n", err))
	}

	if err := UseViper(v); err != nil {
		panic(fmt.Errorf("Fatal error test config file: %s\n", err))
	}

	return
}

// FindConfigFile search in the Paths directories for the file with the given
// name. It returns an error if it cannot find it or if an error occurs while
// searching.
func FindConfigFile(name string) (string, error) {
	for _, cp := range Paths {
		filename := filepath.Join(AbsPath(cp), name)
		ok, err := exists(filename)
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

func exists(name string) (bool, error) {
	_, err := os.Stat(name)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func userHomeDir() string {
	if runtime.GOOS == "windows" {
		home := os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
		return home
	}
	return os.Getenv("HOME")
}

// AbsPath returns an absolute path relative.
func AbsPath(inPath string) string {
	if strings.HasPrefix(inPath, "~") {
		inPath = userHomeDir() + inPath[len("~"):]
	} else if strings.HasPrefix(inPath, "$HOME") {
		inPath = userHomeDir() + inPath[len("$HOME"):]
	}

	if strings.HasPrefix(inPath, "$") {
		end := strings.Index(inPath, string(os.PathSeparator))
		inPath = os.Getenv(inPath[1:end]) + inPath[end:]
	}

	if filepath.IsAbs(inPath) {
		return filepath.Clean(inPath)
	}

	p, err := filepath.Abs(inPath)
	if err == nil {
		return filepath.Clean(p)
	}

	return ""
}
