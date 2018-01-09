package config

import (
	"bytes"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/gomail"
	"github.com/go-redis/redis"
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

var isDevRelease = (BuildMode == "development")

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
var hardcodedRegistry, _ = url.Parse("https://apps-registry.cozy.io/")

// SubdomainType specify how subdomains are structured.
type SubdomainType int

const (
	// FlatSubdomains is the value for apps subdomains like
	// https://<user>-<app>.<domain>/
	FlatSubdomains SubdomainType = iota + 1
	// NestedSubdomains is the value for apps subdomains like
	// https://<app>.<user>.<domain>/ (used by default)
	NestedSubdomains
)

const (
	// SchemeFile is the URL scheme used to configure a file filesystem.
	SchemeFile = "file"
	// SchemeMem is the URL scheme used to configure an in-memory filesystem.
	SchemeMem = "mem"
	// SchemeSwift is the URL scheme used to configure a swift filesystem.
	SchemeSwift = "swift"
)

// defaultAdminSecretFileName is the default name of the file containing the
// administration hashed passphrase.
const defaultAdminSecretFileName = "cozy-admin-passphrase" // #nosec

var config *Config
var log = logger.WithNamespace("config")

// Config contains the configuration values of the application
type Config struct {
	Host                  string
	Port                  int
	Assets                string
	Doctypes              string
	Subdomains            SubdomainType
	AdminHost             string
	AdminPort             int
	AdminSecretFileName   string
	NoReply               string
	Hooks                 string
	GeoDB                 string
	PasswordResetInterval time.Duration

	Fs            Fs
	CouchDB       CouchDB
	Jobs          Jobs
	Konnectors    Konnectors
	Mail          *gomail.DialerOptions
	AutoUpdates   AutoUpdates
	Notifications Notifications
	Logger        logger.Options

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

// Jobs contains the configuration values for the jobs and triggers
// synchronization
type Jobs struct {
	RedisConfig
	NoWorkers bool
	Workers   []Worker
	// XXX for retro-compatibility
	NbWorkers int
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

// Notifications contains the configuration for the mobile push-notification
// center, for Android and iOS
type Notifications struct {
	Development bool

	AndroidAPIKey string

	IOSCertificateKeyPath  string
	IOSCertificatePassword string
	IOSKeyID               string
	IOSTeamID              string
}

// Worker contains the configuration fields for a specific worker type.
type Worker struct {
	WorkerType   string
	Concurrency  *int
	MaxExecCount *int
	Timeout      *time.Duration
}

// RedisConfig contains the configuration values for a redis system
type RedisConfig struct {
	cli redis.UniversalClient
}

// NewRedisConfig creates a redis configuration and its associated client.
func NewRedisConfig(u string) (conf RedisConfig, err error) {
	if u == "" {
		return
	}
	opt, err := redis.ParseURL(u)
	if err != nil {
		return
	}
	conf.cli = redis.NewClient(opt)
	return
}

// GetRedisConfig returns a
func GetRedisConfig(v *viper.Viper, mainOpt *redis.UniversalOptions, key, ptr string) (conf RedisConfig, err error) {
	var localOpt *redis.Options

	localKey := fmt.Sprintf("%s.%s", key, ptr)
	redisKey := fmt.Sprintf("redis.databases.%s", key)

	if u := v.GetString(localKey); u != "" {
		localOpt, err = redis.ParseURL(u)
		if err != nil {
			err = fmt.Errorf("config: can't parse redis URL(%s): %s", u, err)
			return
		}
	}

	if mainOpt != nil && localOpt != nil {
		err = fmt.Errorf("config: ambiguous configuration: the key %q is now "+
			"deprecated and should be removed in favor of %q",
			localKey,
			redisKey)
		return
	}

	if mainOpt != nil {
		opts := *mainOpt
		dbNumber := v.GetString(redisKey)
		if dbNumber == "" {
			err = fmt.Errorf("config: missing DB number for database %q "+
				"in the field %q", key, redisKey)
			return
		}
		opts.DB, err = strconv.Atoi(dbNumber)
		if err != nil {
			err = fmt.Errorf("config: could not parse key %q: %s", redisKey, err)
			return
		}
		conf.cli = redis.NewUniversalClient(&opts)
	} else if localOpt != nil {
		conf.cli = redis.NewClient(localOpt)
	}

	return
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
func (rc *RedisConfig) Client() redis.UniversalClient {
	return rc.cli
}

// IsDevRelease returns whether or not the binary is a development
// release
func IsDevRelease() bool {
	return isDevRelease
}

// GetConfig returns the configured instance of Config
func GetConfig() *Config {
	return config
}

var defaultPasswordResetInterval = 15 * time.Minute

// PasswordResetInterval returns the minimal delay between two password reset
func PasswordResetInterval() time.Duration {
	return config.PasswordResetInterval
}

// Setup Viper to read the environment and the optional config file
func Setup(cfgFile string) (err error) {
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetEnvPrefix("cozy")
	viper.AutomaticEnv()

	viper.SetDefault("password_reset_interval", defaultPasswordResetInterval)


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
	tmpl, err = tmpl.Funcs(numericFuncsMap).ParseFiles(cfgFile)
	if err != nil {
		return fmt.Errorf("Unable to open and parse configuration file "+
			"template %s: %s", cfgFile, err)
	}

	dest := new(bytes.Buffer)
	ctxt := &struct {
		Env    map[string]string
		NumCPU int
	}{
		Env:    envMap(),
		NumCPU: runtime.NumCPU(),
	}
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
	if fsURL.Scheme == "file" {
		fsPath := fsURL.Path
		if fsPath != "" && !path.IsAbs(fsPath) {
			return fmt.Errorf("Filesystem path should be absolute, was: %q", fsPath)
		}
		if fsPath == "/" {
			return fmt.Errorf("Filesystem path should not be root, was: %q", fsPath)
		}
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

	var subdomains SubdomainType
	if subs := v.GetString("subdomains"); subs != "" {
		switch subs {
		case "flat":
			subdomains = FlatSubdomains
		case "nested":
			subdomains = NestedSubdomains
		default:
			return fmt.Errorf(`Subdomains mode should either be "flat" or "nested", was: %q`, subs)
		}
	} else {
		subdomains = NestedSubdomains
	}

	var redisOptions *redis.UniversalOptions
	if v.GetString("redis.addrs") != "" {
		redisOptions = &redis.UniversalOptions{
			// Either a single address or a seed list of host:port addresses
			// of cluster/sentinel nodes.
			Addrs: v.GetStringSlice("redis.addrs"),

			// The sentinel master name.
			// Only failover clients.
			MasterName: v.GetString("redis.master"),

			// Enables read only queries on slave nodes.
			ReadOnly: v.GetBool("redis.read_only_slave"),

			MaxRetries:         v.GetInt("redis.max_retries"),
			Password:           v.GetString("redis.password"),
			DialTimeout:        v.GetDuration("redis.dial_timeout"),
			ReadTimeout:        v.GetDuration("redis.read_timeout"),
			WriteTimeout:       v.GetDuration("redis.write_timeout"),
			PoolSize:           v.GetInt("redis.pool_size"),
			PoolTimeout:        v.GetDuration("redis.pool_timeout"),
			IdleTimeout:        v.GetDuration("redis.idle_timeout"),
			IdleCheckFrequency: v.GetDuration("redis.idle_check_frequency"),
		}
	}

	jobsRedis, err := GetRedisConfig(v, redisOptions, "jobs", "url")
	if err != nil {
		return err
	}
	cacheRedis, err := GetRedisConfig(v, redisOptions, "cache", "url")
	if err != nil {
		return err
	}
	lockRedis, err := GetRedisConfig(v, redisOptions, "lock", "url")
	if err != nil {
		return err
	}
	sessionsRedis, err := GetRedisConfig(v, redisOptions, "sessions", "url")
	if err != nil {
		return err
	}
	downloadRedis, err := GetRedisConfig(v, redisOptions, "downloads", "url")
	if err != nil {
		return err
	}
	konnectorsOauthStateRedis, err := GetRedisConfig(v, redisOptions, "konnectors", "oauthstate")
	if err != nil {
		return err
	}
	realtimeRedis, err := GetRedisConfig(v, redisOptions, "realtime", "url")
	if err != nil {
		return err
	}
	loggerRedis, err := GetRedisConfig(v, redisOptions, "log", "redis")
	if err != nil {
		return err
	}

	adminSecretFile := v.GetString("admin.secret_filename")
	if adminSecretFile == "" {
		adminSecretFile = defaultAdminSecretFileName
	}

	jobs := Jobs{RedisConfig: jobsRedis}
	{
		if nbWorkers := v.GetInt("jobs.workers"); nbWorkers > 0 {
			jobs.NbWorkers = nbWorkers
		} else if ws := v.GetString("jobs.workers"); ws == "false" || ws == "none" || ws == "0" {
			jobs.NoWorkers = true
		} else if workersMap := v.GetStringMap("jobs.workers"); len(workersMap) > 0 {
			workers := make([]Worker, 0, len(workersMap))

			for workerType, mapInterface := range workersMap {
				w := Worker{WorkerType: workerType}

				if enabled, ok := mapInterface.(bool); ok {
					if !enabled {
						zero := 0
						w.Concurrency = &zero
					}
				} else if m, ok := mapInterface.(map[string]interface{}); ok {
					for k, v := range m {
						switch k {
						case "concurrency":
							if concurrency, ok := v.(int); ok {
								w.Concurrency = &concurrency
							}
						case "max_exec_count":
							if maxExecCount, ok := v.(int); ok {
								w.MaxExecCount = &maxExecCount
							}
						case "timeout":
							if timeout, ok := v.(string); ok {
								d, err := time.ParseDuration(timeout)
								if err != nil {
									return fmt.Errorf("config: could not parse timeout duration for worker %q: %s",
										workerType, err)
								}
								w.Timeout = &d
							}
						default:
							return fmt.Errorf("config: unknown key %q",
								"jobs.workers."+workerType+"."+k)
						}
					}
				} else {
					return fmt.Errorf("config: expecting a map in the key %q",
						"jobs.workers."+workerType)
				}

				workers = append(workers, w)
			}
			jobs.Workers = workers
		}
	}

	config = &Config{
		Host:                v.GetString("host"),
		Port:                v.GetInt("port"),
		Subdomains:          subdomains,
		AdminHost:           v.GetString("admin.host"),
		AdminPort:           v.GetInt("admin.port"),
		AdminSecretFileName: adminSecretFile,
		Assets:              v.GetString("assets"),
		Doctypes:            v.GetString("doctypes"),
		NoReply:             v.GetString("mail.noreply_address"),
		Hooks:               v.GetString("hooks"),
		GeoDB:               v.GetString("geodb"),
		PasswordResetInterval: v.GetDuration("password_reset_interval"),

		Fs: Fs{
			URL: fsURL,
		},
		CouchDB: CouchDB{
			Auth: couchAuth,
			URL:  couchURL,
		},
		Jobs: jobs,
		Konnectors: Konnectors{
			Cmd: v.GetString("konnectors.cmd"),
		},
		AutoUpdates: AutoUpdates{
			Activated: v.GetString("auto_updates.schedule") != "",
			Schedule:  v.GetString("auto_updates.schedule"),
		},
		Notifications: Notifications{
			Development: v.GetBool("notifications.development"),

			AndroidAPIKey: v.GetString("notifications.android_api_key"),

			IOSCertificateKeyPath:  v.GetString("notifications.ios_certificate_key_path"),
			IOSCertificatePassword: v.GetString("notifications.ios_certificate_password"),
			IOSKeyID:               v.GetString("notifications.ios_key_id"),
			IOSTeamID:              v.GetString("notifications.ios_team_id"),
		},
		Cache:                       cacheRedis,
		Lock:                        lockRedis,
		SessionStorage:              sessionsRedis,
		DownloadStorage:             downloadRedis,
		KonnectorsOauthStateStorage: konnectorsOauthStateRedis,
		Realtime:                    realtimeRedis,
		Logger: logger.Options{
			Level:  v.GetString("log.level"),
			Syslog: v.GetBool("log.syslog"),
			Redis:  loggerRedis.Client(),
		},
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
	}

	return logger.Init(config.Logger)
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

func createTestViper() *viper.Viper {
	v := viper.New()
	v.SetConfigName("cozy.test")
	v.AddConfigPath("$HOME/.cozy")
	v.SetEnvPrefix("cozy")
	v.AutomaticEnv()
	v.SetDefault("host", "localhost")
	v.SetDefault("port", 8080)
	v.SetDefault("assets", "./assets")
	v.SetDefault("subdomains", "nested")
	v.SetDefault("fs.url", "mem://test")
	v.SetDefault("couchdb.url", "http://localhost:5984/")
	v.SetDefault("cache.url", "redis://localhost:6379/0")
	v.SetDefault("log.level", "info")
	return v
}

// UseTestFile can be used in a test file to inject a configuration
// from a cozy.test.* file. If it can not find this file in your
// $HOME/.cozy directory it will use the default one.
func UseTestFile() {
	v := createTestViper()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			v = createTestViper()
		} else {
			panic(fmt.Errorf("fatal error test config file: %s", err))
		}
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
	return "", fmt.Errorf("Could not find config file %q", name)
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
