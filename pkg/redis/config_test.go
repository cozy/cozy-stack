package redis

import (
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func Test_Config_failover_success(t *testing.T) {
	configMgr := viper.New()
	configMgr.SetConfigType("yaml")
	// A "master" == failover mode
	// Only one addr is required
	configMgr.ReadConfig(strings.NewReader(`
redis:
  addrs: localhost:1234
  pool_size: 33
  master: "ofPuppet"
  password: myDirtyLittleSecret
  databases:
    jobs: 0
    cache: 1
    lock: 2
    sessions: 3
    downloads: 4
    konnectors: 5
    realtime: 6
    log: 7
    rate_limiting: 8
  `))

	var cfg Config
	// Same step as done inside the `config` package
	configMgr.UnmarshalKey("redis", &cfg)

	t.Cleanup(func() { clients = nil })
	err := Init(&cfg)
	assert.NoError(t, err)

	logRedis := GetDB(Log)

	assert.NotNil(t, logRedis)

	redisOpts := logRedis.(*redis.Client).Options()

	// The client connection is handle automatically
	assert.Equal(t, "FailoverClient", redisOpts.Addr)
	// There is a need for a db
	assert.Equal(t, 7, redisOpts.DB)

	assert.Equal(t, 33, redisOpts.PoolSize)
	assert.Equal(t, "myDirtyLittleSecret", redisOpts.Password)
}

func Test_Config_cluster_success(t *testing.T) {
	configMgr := viper.New()
	configMgr.SetConfigType("yaml")
	// Several addrs + no master == Cluster
	configMgr.ReadConfig(strings.NewReader(`
redis:
  addrs: localhost:1234 localhost:4321
  pool_size: 33
  password: myDirtyLittleSecret
  databases:
    jobs: 0
    cache: 1
    lock: 2
    sessions: 3
    downloads: 4
    konnectors: 5
    realtime: 6
    log: 7
    rate_limiting: 8
  `))

	var cfg Config
	// Same step as done inside the `config` package
	configMgr.UnmarshalKey("redis", &cfg)

	t.Cleanup(func() { clients = nil })
	err := Init(&cfg)
	assert.NoError(t, err)

	lockRedis := GetDB("lock")

	assert.NotNil(t, lockRedis)

	redisOpts := lockRedis.(*redis.ClusterClient).Options()

	// There is no use of DB for cluster
	assert.Equal(t, 33, redisOpts.PoolSize)
	assert.Equal(t, "myDirtyLittleSecret", redisOpts.Password)
	assert.Equal(t, []string{"localhost:1234", "localhost:4321"}, redisOpts.Addrs)
}

func Test_Config_with_no_redis(t *testing.T) {
	configMgr := viper.New()
	configMgr.SetConfigType("yaml")
	// No redis config
	configMgr.ReadConfig(strings.NewReader(`
redis:
  `))

	var cfg Config
	// Same step as done inside the `config` package
	configMgr.UnmarshalKey("redis", &cfg)

	t.Cleanup(func() { clients = nil })
	err := Init(&cfg)
	assert.NoError(t, err)

	lockRedis := GetDB("lock")
	assert.Nil(t, lockRedis)
}

func Test_Config_addrs_with_spaces(t *testing.T) {
	configMgr := viper.New()
	configMgr.SetConfigType("yaml")
	configMgr.ReadConfig(strings.NewReader(`
redis:
  addrs: localhost:1234 localhost:4321
  master: ofPuppet
  pool_size: 33
  password: myDirtyLittleSecret
  databases:
    jobs: 0
    cache: 1
    lock: 2
    sessions: 3
    downloads: 4
    konnectors: 5
    realtime: 6
    log: 7
    rate_limiting: 8
  `))

	var cfg Config
	// Same step as done inside the `config` package
	configMgr.UnmarshalKey("redis", &cfg)

	t.Cleanup(func() { clients = nil })
	err := Init(&cfg)
	assert.NoError(t, err)

	assert.Equal(t, []string{"localhost:1234", "localhost:4321"}, cfg.Addrs)
}

func Test_Config_addrs_with_spaces_mixed(t *testing.T) {
	configMgr := viper.New()
	configMgr.SetConfigType("yaml")
	configMgr.ReadConfig(strings.NewReader(`
redis:
  addrs: ["localhost:1 localhost:2", "localhost:3", "localhost:4"]
  master: ofPuppet
  pool_size: 33
  password: myDirtyLittleSecret
  databases:
    jobs: 0
    cache: 1
    lock: 2
    sessions: 3
    downloads: 4
    konnectors: 5
    realtime: 6
    log: 7
    rate_limiting: 8
  `))

	var cfg Config
	// Same step as done inside the `config` package
	configMgr.UnmarshalKey("redis", &cfg)

	t.Cleanup(func() { clients = nil })
	err := Init(&cfg)
	assert.NoError(t, err)

	assert.Equal(t, []string{"localhost:1", "localhost:2", "localhost:3", "localhost:4"}, cfg.Addrs)
}

func Test_Config_with_a_missing_database_key(t *testing.T) {
	configMgr := viper.New()
	configMgr.SetConfigType("yaml")
	configMgr.ReadConfig(strings.NewReader(`
redis:
  addrs: localhost:1234 localhost:4321
  master: ofPuppet
  pool_size: 33
  password: myDirtyLittleSecret
  databases:
    jobs: 0
    cache: 1
    lock: 2
    sessions: 3
    downloads: 4
    konnectors: 5
    realtime: 6
    rate_limiting: 8
  `)) // missing "log

	var cfg Config
	// Same step as done inside the `config` package
	configMgr.UnmarshalKey("redis", &cfg)

	err := Init(&cfg)
	assert.EqualError(t, err, `missing DB number for database "log"`)
}

func Test_Config_with_a_little_pool_size(t *testing.T) {
	configMgr := viper.New()
	configMgr.SetConfigType("yaml")
	configMgr.ReadConfig(strings.NewReader(`
redis:
  addrs: localhost:1234 localhost:4321
  master: ofPuppet
  pool_size: 4
  password: myDirtyLittleSecret
  databases:
    jobs: 0
    cache: 1
    lock: 2
    sessions: 3
    downloads: 4
    konnectors: 5
    realtime: 6
    log: 7
    rate_limiting: 8
  `))

	var cfg Config
	// Same step as done inside the `config` package
	configMgr.UnmarshalKey("redis", &cfg)

	t.Cleanup(func() { clients = nil })
	err := Init(&cfg)
	assert.NoError(t, err)

	// The configured value vary depending the CPU number so we can't
	// check for a specific number.
	assert.True(t, cfg.PoolSize >= minPoolSize)
}

func Test_Config_get_db_without_init(t *testing.T) {
	assert.Panics(t, func() { GetDB(Lock) })
}
