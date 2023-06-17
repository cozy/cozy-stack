package redis

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Database string

type Client redis.UniversalClient

const (
	Jobs         Database = "jobs"
	Cache        Database = "cache"
	Lock         Database = "lock"
	Sessions     Database = "sessions"
	Downloads    Database = "downloads"
	Konnectors   Database = "konnectors"
	Realtime     Database = "realtime"
	Log          Database = "log"
	RateLimiting Database = "rate_limiting"
)

// mandatoryDBs list all the DB keys required inside the config in order to
// boot.
var mandatoryDBs = []Database{Jobs, Cache, Lock, Sessions, Downloads, Konnectors, Realtime, Log, RateLimiting}

var clients map[Database]redis.UniversalClient

type Config struct {
	// Either a single address or a seed list of host:port addresses
	// of cluster/sentinel nodes.
	Addrs []string `mapstructure:"addrs"`
	// The sentinel master name. Only failover clients.
	MasterName string `mapstructure:"master"`
	// Enables read only queries on slave nodes.
	ReadOnly bool `mapstructure:"read_only_slave"`
	// redis password
	Password string `mapstructure:"password"`
	// databases number for each part of the stack using a specific database.
	Databases map[Database]int `mapstructure:"databases"`

	MaxRetries int `mapstructure:"max_retries"`

	// Advanced configs
	DialTimeout     time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	PoolSize        int           `mapstructure:"pool_size"`
	PoolTimeout     time.Duration `mapstructure:"pool_timeout"`
	ConnMaxIdleTime time.Duration `mapstructure:"idle_timeout"`
}

const minPoolSize = 25

func Init(cfg *Config) error {
	clients = map[Database]redis.UniversalClient{}

	if len(cfg.Addrs) == 0 {
		return nil
	}

	parseAddrs(cfg)

	if cfg.PoolSize < minPoolSize {
		// Default go-redis pool size is 10 * runtime.NumCPU() which is
		// too short on a single-cpu server, we consume at leat 19 connections,
		// so we enforce a minimum of 25, keeping the default 10 * runtime*NumCPU()
		// if larger
		defaultPoolSize := max(minPoolSize, 10*runtime.NumCPU())
		cfg.PoolSize = defaultPoolSize
	}

	for _, db := range mandatoryDBs {
		dbNumber, ok := cfg.Databases[db]
		if !ok {
			return fmt.Errorf("missing DB number for database %q", db)
		}

		client := redis.NewUniversalClient(&redis.UniversalOptions{
			Addrs:           cfg.Addrs,
			DB:              dbNumber,
			MasterName:      cfg.MasterName,
			ReadOnly:        cfg.ReadOnly,
			Password:        cfg.Password,
			MaxRetries:      cfg.MaxRetries,
			DialTimeout:     cfg.DialTimeout,
			ReadTimeout:     cfg.ReadTimeout,
			WriteTimeout:    cfg.WriteTimeout,
			PoolSize:        cfg.PoolSize,
			PoolTimeout:     cfg.PoolTimeout,
			ConnMaxIdleTime: cfg.ConnMaxIdleTime,
		})

		clients[db] = client
	}

	return nil
}

func GetDB(db Database) redis.UniversalClient {
	if clients == nil {
		panic("You must call redis.Init before calling redis.GetDB")
	}

	return clients[db]
}

// math.Max() is a float64 function, so we define int function here
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// parseAddrs will normalize all the `cfg.Addr` content.
// Each string in [Config.Addrs] can be either an url or
// several urls separated by spaces.
func parseAddrs(cfg *Config) {
	res := []string{}

	for _, addr := range cfg.Addrs {
		addrs := strings.Fields(addr)
		if len(addrs) == 0 {
			res = append(res, addr)
			return
		}

		res = append(res, addrs...)
	}

	cfg.Addrs = res
}
