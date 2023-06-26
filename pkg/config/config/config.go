package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/cozy/cozy-stack/pkg/avatar"
	"github.com/cozy/cozy-stack/pkg/cache"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/keyring"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/pkg/lock"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/tlsclient"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/gomail"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
)

// DefaultInstanceContext is the default context name for an instance
const DefaultInstanceContext = "default"

// Filename is the default configuration filename that cozy
// search for
const Filename = "cozy"

// Paths is the list of directories used to search for a
// configuration file
var Paths = []string{
	".",
	".cozy",
	"$HOME/.cozy",
	"$HOME/.config/cozy",
	"$XDG_CONFIG_HOME/cozy",
	"/etc/cozy",
}

// hardcodedRegistry is the default registry used if no configuration is set
// for registries.
var hardcodedRegistry, _ = url.Parse("https://apps-registry.cozycloud.cc/")

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
	// SchemeSwiftSecure is the URL scheme used to configure the swift filesystem
	// in secure mode (HTTPS).
	SchemeSwiftSecure = "swift+https"
)

// defaultAdminSecretFileName is the default name of the file containing the
// administration hashed passphrase.
const defaultAdminSecretFileName = "cozy-admin-passphrase"

var (
	config *Config
)

var log = logger.WithNamespace("config")

// Config contains the configuration values of the application
type Config struct {
	Host string
	Port int

	AdminHost           string
	AdminPort           int
	AdminSecretFileName string

	Assets                string
	Doctypes              string
	Subdomains            SubdomainType
	AlertAddr             string
	NoReplyAddr           string
	NoReplyName           string
	ReplyTo               string
	Hooks                 string
	GeoDB                 string
	PasswordResetInterval time.Duration

	RemoteAssets   map[string]string
	DeprecatedApps DeprecatedAppsCfg

	Avatars        *avatar.Service
	Fs             Fs
	Keyring        keyring.Keyring
	CouchDB        CouchDB
	Jobs           Jobs
	Konnectors     Konnectors
	Mail           *gomail.DialerOptions
	MailPerContext map[string]interface{}
	Move           Move
	Notifications  Notifications
	Flagship       Flagship

	Lock              lock.Getter
	Limiter           *limits.RateLimiter
	SessionStorage    redis.UniversalClient
	DownloadStorage   redis.UniversalClient
	OauthStateStorage redis.UniversalClient
	Realtime          redis.UniversalClient

	CacheStorage cache.Cache

	Contexts       map[string]interface{}
	Authentication map[string]interface{}
	Office         map[string]Office
	Registries     map[string][]*url.URL
	Clouderies     map[string]ClouderyConfig

	RemoteAllowCustomPort bool

	CSPDisabled   bool
	CSPAllowList  map[string]string
	CSPPerContext map[string]map[string]string

	AssetsPollingDisabled bool
	AssetsPollingInterval time.Duration
}

// ClouderyConfig for [cloudery.ClouderyService].
type ClouderyConfig struct {
	API struct {
		URL   string `mapstructure:"url"`
		Token string `mapstructure:"token"`
	} `mapstructure:"api"`
}

// Fs contains the configuration values of the file-system
type Fs struct {
	Auth                  *url.Userinfo
	URL                   *url.URL
	Transport             http.RoundTripper
	DefaultLayout         int
	CanQueryInfo          bool
	AutoCleanTrashedAfter map[string]string
	Versioning            FsVersioning
	Contexts              map[string]interface{}
}

// FsVersioning contains the configuration for the versioning of files
type FsVersioning struct {
	MaxNumberToKeep            int
	MinDelayBetweenTwoVersions time.Duration
}

// CouchDBCluster contains the configuration values for a cluster of CouchDB.
type CouchDBCluster struct {
	Auth     *url.Userinfo
	URL      *url.URL
	Creation bool
}

// CouchDB contains the configuration for the CouchDB clusters.
type CouchDB struct {
	Client   *http.Client
	Global   CouchDBCluster
	Clusters []CouchDBCluster
}

// Jobs contains the configuration values for the jobs and triggers
// synchronization
type Jobs struct {
	Client                redis.UniversalClient
	NoWorkers             bool
	AllowList             bool
	Workers               []Worker
	ImageMagickConvertCmd string
	// XXX for retro-compatibility
	NbWorkers             int
	DefaultDurationToKeep string
}

// Konnectors contains the configuration values for the konnectors
type Konnectors struct {
	Cmd string
}

// Move contains the configuration for the move wizard
type Move struct {
	URL string
}

// Office contains the configuration for collaborative edition of office
// documents
type Office struct {
	OnlyOfficeURL string
	InboxSecret   string
	OutboxSecret  string
}

// Notifications contains the configuration for the mobile push-notification
// center, for Android and iOS
type Notifications struct {
	Development bool

	AndroidAPIKey string
	FCMServer     string

	IOSCertificateKeyPath  string
	IOSCertificatePassword string
	IOSKeyID               string
	IOSTeamID              string

	HuaweiGetTokenURL     string
	HuaweiSendMessagesURL string

	Contexts map[string]SMS
}

// Flagship contains the configuration for the flagship app.
type Flagship struct {
	Contexts              map[string]interface{}
	APKPackageNames       []string
	APKCertificateDigests []string
	AppleAppIDs           []string
}

// SMS contains the configuration to send notifications by SMS.
type SMS struct {
	Provider string
	URL      string
	Token    string
}

// DeprecatedCfg describes the config used to setup [github.com/cozy/cozy-stack/web/auth.DeprecatedAppList].
//
// XXX: Move this struct next to [github.com/cozy/cozy-stack/web/auth.DeprecatedAppList]
// once the circling dependency issue is fixed.
type DeprecatedAppsCfg struct {
	Apps []DeprecatedApp `mapstructure:"apps"`
}

// DeprecatedApp describes a list deprecated app and the links used to replace them.
type DeprecatedApp struct {
	// SoftwareID found inside the oauth client.
	SoftwareID string `mapstructure:"software_id"`
	// Name as printed to the user.
	Name string `mapstructure:"name"`

	StoreURLs map[string]string `mapstructure:"store_urls"`
}

// Worker contains the configuration fields for a specific worker type.
type Worker struct {
	WorkerType   string
	Concurrency  *int
	MaxExecCount *int
	Timeout      *time.Duration
}

// GetRedis returns a [redis.UniversalClient] for the given db.
func GetRedis(v *viper.Viper, mainOpt *redis.UniversalOptions, key, ptr string) (redis.UniversalClient, error) {
	var localOpt *redis.Options
	var err error

	localKey := fmt.Sprintf("%s.%s", key, ptr)

	if u := v.GetString(localKey); u != "" {
		localOpt, err = redis.ParseURL(u)
		if err != nil {
			return nil, fmt.Errorf("config: can't parse redis URL(%s): %s", u, err)
		}
	}

	if mainOpt == nil && localOpt == nil {
		return nil, nil
	}

	if mainOpt != nil && localOpt != nil {
		return nil, fmt.Errorf("config: ambiguous configuration between the cli and the config")
	}

	if localOpt != nil {
		return redis.NewClient(localOpt), nil
	}

	redisKey := fmt.Sprintf("redis.databases.%s", key)

	opts := *mainOpt
	dbNumber := v.GetString(redisKey)
	if dbNumber == "" {
		return nil, fmt.Errorf("config: missing DB number for database %q "+"in the field %q", key, redisKey)
	}
	opts.DB, err = strconv.Atoi(dbNumber)
	if err != nil {
		return nil, fmt.Errorf("config: could not parse key %q: %s", redisKey, err)
	}

	return redis.NewUniversalClient(&opts), nil
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

// CouchCluster returns the CouchDB configuration for the given cluster.
func CouchCluster(n int) CouchDBCluster {
	if 0 <= n && n < len(config.CouchDB.Clusters) {
		return config.CouchDB.Clusters[n]
	}
	return config.CouchDB.Global
}

// CouchClient returns the http client to use when making requests to a CouchDB
// cluster.
func CouchClient() *http.Client {
	return config.CouchDB.Client
}

// Lock return the lock getter.
func Lock() lock.Getter {
	return config.Lock
}

// GetConfig returns the configured instance of Config
func GetConfig() *Config {
	return config
}

// Avatars return the configured initials service.
func Avatars() *avatar.Service {
	return config.Avatars
}

// GetKeyring returns the configured instance of [keyring.Keyring]
func GetKeyring() keyring.Keyring {
	return config.Keyring
}

// GetRateLimiter return the setup rate limiter.
func GetRateLimiter() *limits.RateLimiter {
	return config.Limiter
}

// GetOIDC returns the OIDC config for the given context (with a boolean to say
// if OIDC is enabled).
func GetOIDC(contextName string) (map[string]interface{}, bool) {
	if contextName == "" {
		return nil, false
	}
	auth, ok := config.Authentication[contextName].(map[string]interface{})
	if !ok {
		return nil, false
	}
	config, ok := auth["oidc"].(map[string]interface{})
	return config, ok
}

// GetFranceConnect returns the FranceConnect config for the given context
// (with a boolean to say if FranceConnect is enabled).
func GetFranceConnect(contextName string) (map[string]interface{}, bool) {
	if contextName == "" {
		return nil, false
	}
	auth, ok := config.Authentication[contextName].(map[string]interface{})
	if !ok {
		return nil, false
	}
	config, ok := auth["franceconnect"].(map[string]interface{})
	return config, ok
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
	applyDefaults(viper.GetViper())

	var cfgFiles []string
	if cfgFile == "" {
		cfgFiles, err = findConfigFiles(Filename)
		if err != nil {
			return err
		}
	} else {
		cfgFiles = []string{cfgFile}
	}

	if len(cfgFiles) == 0 {
		return UseViper(viper.GetViper())
	}

	log.Debugf("Using config files: %s", cfgFiles)

	for _, cfgFile = range cfgFiles {
		tmplName := filepath.Base(cfgFile)
		tmpl := template.New(tmplName)
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
		err = tmpl.ExecuteTemplate(dest, tmplName, ctxt)
		if err != nil {
			return fmt.Errorf("Template error for config files %s: %s", cfgFile, err)
		}

		cfgFile = regexp.MustCompile(`\.local$`).ReplaceAllString(cfgFile, "")
		if ext := filepath.Ext(cfgFile); len(ext) > 0 {
			viper.SetConfigType(ext[1:])
		}
		if err := viper.MergeConfig(dest); err != nil {
			if _, isParseErr := err.(viper.ConfigParseError); isParseErr {
				log.Errorf("Failed to read cozy-stack configurations from %s", cfgFile)
				log.Errorf(dest.String())
				return err
			}
		}
	}

	return UseViper(viper.GetViper())
}

func applyDefaults(v *viper.Viper) {
	v.SetDefault("password_reset_interval", defaultPasswordResetInterval)
	v.SetDefault("jobs.imagemagick_convert_cmd", "convert")
	v.SetDefault("jobs.defaultDurationToKeep", "2W")
	v.SetDefault("assets_polling_disabled", false)
	v.SetDefault("assets_polling_interval", 2*time.Minute)
	v.SetDefault("fs.versioning.max_number_of_versions_to_keep", 20)
	v.SetDefault("fs.versioning.min_delay_between_two_versions", 15*time.Minute)
}

func envMap() map[string]string {
	env := make(map[string]string)
	for _, i := range os.Environ() {
		sep := strings.Index(i, "=")
		env[i[0:sep]] = i[sep+1:]
	}
	return env
}

// math.Max() is a float64 function, so we define int function here
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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

	couch, err := makeCouch(v)
	if err != nil {
		return err
	}

	fsClient, _, err := tlsclient.NewHTTPClient(tlsclient.HTTPEndpoint{
		RootCAFile: v.GetString("fs.root_ca"),
		ClientCertificateFiles: tlsclient.ClientCertificateFilePair{
			CertificateFile: v.GetString("fs.client_cert"),
			KeyFile:         v.GetString("fs.client_key"),
		},
		PinnedKey:              v.GetString("fs.pinned_key"),
		InsecureSkipValidation: v.GetBool("fs.insecure_skip_validation"),
		MaxIdleConnsPerHost:    128,
		DisableCompression:     true,
	})
	if err != nil {
		return err
	}

	regs, err := makeRegistries(v)
	if err != nil {
		return err
	}

	office, err := makeOffice(v)
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
		// Default go-redis pool size is 10 * runtime.NumCPU() which is
		// too short on a single-cpu server, we consume at leat 19 connections,
		// so we enforce a minimum of 25, keeping the default 10 * runtime*NumCPU()
		// if larger
		redisPoolSize := v.GetInt("redis.pool_size")
		if redisPoolSize < 25 {
			if redisPoolSize != 0 {
				log.Warnf("Redis pool size set smaller than 25. Using default value.")
			}
			redisPoolSize = max(25, 10*runtime.NumCPU())
		}

		redisOptions = &redis.UniversalOptions{
			// Either a single address or a seed list of host:port addresses
			// of cluster/sentinel nodes.
			Addrs: v.GetStringSlice("redis.addrs"),

			// The sentinel master name.
			// Only failover clients.
			MasterName: v.GetString("redis.master"),

			// Enables read only queries on slave nodes.
			ReadOnly: v.GetBool("redis.read_only_slave"),

			MaxRetries:      v.GetInt("redis.max_retries"),
			Password:        v.GetString("redis.password"),
			DialTimeout:     v.GetDuration("redis.dial_timeout"),
			ReadTimeout:     v.GetDuration("redis.read_timeout"),
			WriteTimeout:    v.GetDuration("redis.write_timeout"),
			PoolSize:        redisPoolSize,
			PoolTimeout:     v.GetDuration("redis.pool_timeout"),
			ConnMaxIdleTime: v.GetDuration("redis.idle_timeout"),
		}
	}

	jobsRedis, err := GetRedis(v, redisOptions, "jobs", "url")
	if err != nil {
		return err
	}
	lockRedis, err := GetRedis(v, redisOptions, "lock", "url")
	if err != nil {
		return err
	}
	sessionsRedis, err := GetRedis(v, redisOptions, "sessions", "url")
	if err != nil {
		return err
	}
	downloadRedis, err := GetRedis(v, redisOptions, "downloads", "url")
	if err != nil {
		return err
	}
	rateLimitingRedis, err := GetRedis(v, redisOptions, "rate_limiting", "url")
	if err != nil {
		return err
	}
	oauthStateRedis, err := GetRedis(v, redisOptions, "konnectors", "oauthstate")
	if err != nil {
		return err
	}
	realtimeRedis, err := GetRedis(v, redisOptions, "realtime", "url")
	if err != nil {
		return err
	}
	loggerRedis, err := GetRedis(v, redisOptions, "log", "redis")
	if err != nil {
		return err
	}

	// cache entry is optional
	cacheRedis, _ := GetRedis(v, redisOptions, "cache", "url")

	adminSecretFile := v.GetString("admin.secret_filename")
	if adminSecretFile == "" {
		adminSecretFile = defaultAdminSecretFileName
	}

	jobs := Jobs{
		Client:                jobsRedis,
		ImageMagickConvertCmd: v.GetString("jobs.imagemagick_convert_cmd"),
		DefaultDurationToKeep: v.GetString("jobs.defaultDurationToKeep"),
	}
	{
		if allow := v.GetBool("jobs.allowlist"); allow {
			jobs.AllowList = true
		}
		if allow := v.GetBool("jobs.whitelist"); allow { // For compatibility
			jobs.AllowList = true
		}
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
								var d time.Duration
								d, err = time.ParseDuration(timeout)
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

	// Use the layout v3 (value 2) for missing/invalid value
	defaultLayout := 2
	if v.Get("fs.default_layout") != "" {
		layout := v.GetInt("fs.default_layout")
		if 0 <= layout && layout <= 2 {
			defaultLayout = layout
		}
	}

	cspAllowList := map[string]string{}
	cspPerContext := map[string]map[string]string{}
	cspList := v.GetStringMap("csp_allowlist")
	for key, value := range cspList {
		if val, ok := value.(string); ok {
			cspAllowList[key] = val
		} else if val, ok := value.(map[string]interface{}); ok && key == "contexts" {
			for ctx, rules := range val {
				if rule, ok := rules.(map[string]interface{}); ok {
					forContext := map[string]string{}
					for src, list := range rule {
						if l, ok := list.(string); ok {
							forContext[src] = l
						}
					}
					cspPerContext[ctx] = forContext
				}
			}
		}
	}

	cacheStorage := cache.New(cacheRedis)

	avatars, err := avatar.NewService(cacheStorage, v.GetString("jobs.imagemagick_convert_cmd"))
	if err != nil {
		return fmt.Errorf("failed to create the avatar service: %w", err)
	}

	// Setup keyring
	var keyringCfg keyring.Config
	err = v.UnmarshalKey("vault", &keyringCfg)
	if err != nil {
		return fmt.Errorf("failed to decode the vault config: %w", err)
	}
	keyring, err := keyring.NewFromConfig(keyringCfg)
	if err != nil {
		return fmt.Errorf("failed to setup the keyring: %w", err)
	}

	config = &Config{
		Host: v.GetString("host"),
		Port: v.GetInt("port"),

		AdminHost:           v.GetString("admin.host"),
		AdminPort:           v.GetInt("admin.port"),
		AdminSecretFileName: adminSecretFile,

		Subdomains:            subdomains,
		Assets:                v.GetString("assets"),
		Doctypes:              v.GetString("doctypes"),
		AlertAddr:             v.GetString("mail.alert_address"),
		NoReplyAddr:           v.GetString("mail.noreply_address"),
		NoReplyName:           v.GetString("mail.noreply_name"),
		ReplyTo:               v.GetString("mail.reply_to"),
		Hooks:                 v.GetString("hooks"),
		GeoDB:                 v.GetString("geodb"),
		PasswordResetInterval: v.GetDuration("password_reset_interval"),

		RemoteAssets: v.GetStringMapString("remote_assets"),

		Avatars: avatars,
		Keyring: keyring,
		Fs: Fs{
			URL:                   fsURL,
			Transport:             fsClient.Transport,
			DefaultLayout:         defaultLayout,
			CanQueryInfo:          v.GetBool("fs.can_query_info"),
			AutoCleanTrashedAfter: v.GetStringMapString("fs.auto_clean_trashed_after"),
			Versioning: FsVersioning{
				MaxNumberToKeep:            v.GetInt("fs.versioning.max_number_of_versions_to_keep"),
				MinDelayBetweenTwoVersions: v.GetDuration("fs.versioning.min_delay_between_two_versions"),
			},
			Contexts: v.GetStringMap("fs.contexts"),
		},
		CouchDB: couch,
		Jobs:    jobs,
		Konnectors: Konnectors{
			Cmd: v.GetString("konnectors.cmd"),
		},
		Move: Move{
			URL: v.GetString("move.url"),
		},
		Notifications: Notifications{
			Development: v.GetBool("notifications.development"),

			FCMServer:     v.GetString("notifications.fcm_server"),
			AndroidAPIKey: v.GetString("notifications.android_api_key"),

			IOSCertificateKeyPath:  v.GetString("notifications.ios_certificate_key_path"),
			IOSCertificatePassword: v.GetString("notifications.ios_certificate_password"),
			IOSKeyID:               v.GetString("notifications.ios_key_id"),
			IOSTeamID:              v.GetString("notifications.ios_team_id"),

			HuaweiGetTokenURL:     v.GetString("notifications.huawei_get_token"),
			HuaweiSendMessagesURL: v.GetString("notifications.huawei_send_message"),

			Contexts: makeSMS(v.GetStringMap("notifications.contexts")),
		},
		Flagship: Flagship{
			Contexts:              v.GetStringMap("flagship.contexts"),
			APKPackageNames:       v.GetStringSlice("flagship.apk_package_names"),
			APKCertificateDigests: v.GetStringSlice("flagship.apk_certificate_digests"),
			AppleAppIDs:           v.GetStringSlice("flagship.apple_app_ids"),
		},
		Lock:              lock.New(lockRedis),
		SessionStorage:    sessionsRedis,
		DownloadStorage:   downloadRedis,
		Limiter:           limits.NewRateLimiter(rateLimitingRedis),
		OauthStateStorage: oauthStateRedis,
		Realtime:          realtimeRedis,
		CacheStorage:      cacheStorage,
		Mail: &gomail.DialerOptions{
			Host:                      v.GetString("mail.host"),
			Port:                      v.GetInt("mail.port"),
			Username:                  v.GetString("mail.username"),
			Password:                  v.GetString("mail.password"),
			DisableTLS:                v.GetBool("mail.disable_tls"),
			SkipCertificateValidation: v.GetBool("mail.skip_certificate_validation"),
		},
		MailPerContext: v.GetStringMap("mail.contexts"),
		Contexts:       v.GetStringMap("contexts"),
		Authentication: v.GetStringMap("authentication"),
		Office:         office,
		Registries:     regs,

		CSPAllowList:  cspAllowList,
		CSPPerContext: cspPerContext,

		AssetsPollingDisabled: v.GetBool("assets_polling_disabled"),
		AssetsPollingInterval: v.GetDuration("assets_polling_interval"),
	}

	err = v.UnmarshalKey("deprecated_apps", &config.DeprecatedApps)
	if err != nil {
		return fmt.Errorf(`failed to parse the config for "deprecated_apps": %w`, err)
	}

	err = v.UnmarshalKey("clouderies", &config.Clouderies)
	if err != nil {
		return fmt.Errorf(`failed to parse the config for "deprecated_apps": %w`, err)
	}

	// For compatibility
	if len(config.CSPAllowList) == 0 {
		config.CSPAllowList = v.GetStringMapString("csp_whitelist")
	}

	if build.IsDevRelease() && v.GetBool("disable_csp") {
		config.CSPDisabled = true
	}

	if v.GetBool("remote_allow_custom_port") {
		config.RemoteAllowCustomPort = true
	}

	loggerOpts := logger.Options{
		Level: v.GetString("log.level"),
		Redis: loggerRedis,
	}

	if v.GetBool("log.syslog") {
		syslogHook, err := logger.SyslogHook()
		if err != nil {
			return fmt.Errorf("failed to setup the syslog hook: %w", err)
		}

		// Redirect all the logs to the syslog hook and don't log to STDIO
		loggerOpts.Hooks = append(loggerOpts.Hooks, syslogHook)
		loggerOpts.Output = io.Discard
	}

	if err = logger.Init(loggerOpts); err != nil {
		return err
	}

	return nil
}

func makeCouch(v *viper.Viper) (CouchDB, error) {
	var couch CouchDB
	couchClient, _, err := tlsclient.NewHTTPClient(tlsclient.HTTPEndpoint{
		Timeout:             10 * time.Second,
		MaxIdleConnsPerHost: 20,
		RootCAFile:          v.GetString("couchdb.root_ca"),
		ClientCertificateFiles: tlsclient.ClientCertificateFilePair{
			CertificateFile: v.GetString("couchdb.client_cert"),
			KeyFile:         v.GetString("couchdb.client_key"),
		},
		PinnedKey:              v.GetString("couchdb.pinned_key"),
		InsecureSkipValidation: v.GetBool("couchdb.insecure_skip_validation"),
	})
	if err != nil {
		return couch, err
	}
	couch.Client = couchClient

	couchURL, couchAuth, err := parseURL(v.GetString("couchdb.url"))
	if err != nil {
		return couch, err
	}
	if couchURL.Path == "" {
		couchURL.Path = "/"
	}
	couch.Global = CouchDBCluster{
		Auth:     couchAuth,
		URL:      couchURL,
		Creation: true,
	}

	if clusters, ok := v.Get("couchdb.clusters").([]interface{}); ok {
		for _, cluster := range clusters {
			cluster, _ := cluster.(map[string]interface{})
			u, _ := cluster["url"].(string)
			couchURL, couchAuth, err := parseURL(u)
			if err != nil {
				return couch, err
			}
			if couchURL.Path == "" {
				couchURL.Path = "/"
			}
			creation := true
			if c, ok := cluster["instance_creation"].(bool); ok {
				creation = c
			}
			couch.Clusters = append(couch.Clusters, CouchDBCluster{
				Auth:     couchAuth,
				URL:      couchURL,
				Creation: creation,
			})
		}
	}

	if len(couch.Clusters) == 0 {
		couch.Clusters = []CouchDBCluster{couch.Global}
	}
	return couch, nil
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
		regs[DefaultInstanceContext] = urlList
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

	defaults, ok := regs[DefaultInstanceContext]
	if !ok {
		defaults = []*url.URL{hardcodedRegistry}
		regs[DefaultInstanceContext] = defaults
	}
	for ctx, urls := range regs {
		if ctx == DefaultInstanceContext {
			continue
		}
		regs[ctx] = append(urls, defaults...)
	}

	return regs, nil
}

func makeOffice(v *viper.Viper) (map[string]Office, error) {
	office := make(map[string]Office)
	for k, v := range v.GetStringMap("office") {
		ctx, ok := v.(map[string]interface{})
		if !ok {
			return nil, errors.New("Bad format in the office section of the configuration file")
		}
		url, ok := ctx["onlyoffice_url"].(string)
		if !ok {
			return nil, errors.New("Bad format in the office section of the configuration file")
		}
		inbox, _ := ctx["onlyoffice_inbox_secret"].(string)
		outbox, _ := ctx["onlyoffice_outbox_secret"].(string)
		office[k] = Office{
			OnlyOfficeURL: url,
			InboxSecret:   inbox,
			OutboxSecret:  outbox,
		}
	}

	if url := v.GetString("office.default.onlyoffice_url"); url != "" {
		office[DefaultInstanceContext] = Office{
			OnlyOfficeURL: url,
			InboxSecret:   v.GetString("office.default.onlyoffice_inbox_secret"),
			OutboxSecret:  v.GetString("office.default.onlyoffice_outbox_secret"),
		}
	}

	return office, nil
}

func makeSMS(raw map[string]interface{}) map[string]SMS {
	sms := make(map[string]SMS)
	for name, val := range raw {
		entry, ok := val.(map[string]interface{})
		if !ok {
			continue
		}
		provider, _ := entry["provider"].(string)
		if provider == "" {
			continue
		}
		url, _ := entry["url"].(string)
		token, _ := entry["token"].(string)
		sms[name] = SMS{Provider: provider, URL: url, Token: token}
	}
	return sms
}

func createTestViper() *viper.Viper {
	v := viper.New()
	v.SetConfigName("cozy.test")
	v.AddConfigPath("$HOME/.cozy")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.SetEnvPrefix("cozy")
	v.AutomaticEnv()
	v.SetDefault("host", "localhost")
	v.SetDefault("port", 8080)
	v.SetDefault("assets", "./assets")
	v.SetDefault("subdomains", "nested")
	v.SetDefault("fs.url", "mem://test")
	v.SetDefault("couchdb.url", "http://localhost:5984/")
	v.SetDefault("log.level", "info")
	v.SetDefault("assets_polling_disabled", false)
	v.SetDefault("assets_polling_interval", 2*time.Minute)
	applyDefaults(v)
	return v
}

// UseTestFile can be used in a test file to inject a configuration
// from a cozy.test.* file. If it can not find this file in your
// $HOME/.cozy directory it will use the default one.
func UseTestFile(t *testing.T) {
	t.Helper()

	build.BuildMode = build.ModeProd
	v := createTestViper()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			v = createTestViper()
		} else {
			t.Fatalf("fatal error test config file: %s", err)
		}
	}

	if err := UseViper(v); err != nil {
		t.Fatalf("fatal error test config file: %s", err)
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

// findConfigFiles search in the Paths directories for the first existing directory,
// then look for supported Viper file for both .ext and .ext.local version, the later
// taking precedence.
func findConfigFiles(name string) ([]string, error) {
	var configFiles []string
	configFile := ""
	for _, ext := range viper.SupportedExts {
		configFile, _ = FindConfigFile(name + "." + ext)
		if configFile != "" {
			break
		}
	}
	if configFile == "" {
		return nil, nil
	}

	configFiles = append(configFiles, configFile)

	configFile += ".local"
	ok, _ := utils.FileExists(configFile)
	if ok {
		configFiles = append(configFiles, configFile)
	}

	return configFiles, nil
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
