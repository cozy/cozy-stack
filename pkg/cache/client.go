package cache

import (
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/go-redis/redis"
)

type subRedisInterface interface {
	Get(string) *redis.StringCmd
	Set(string, interface{}, time.Duration) *redis.StatusCmd
	Del(...string) *redis.IntCmd
}

var mu sync.Mutex
var globalRedisClient subRedisInterface

func getClient() subRedisInterface {
	mu.Lock()
	defer mu.Unlock()

	if globalRedisClient != nil {
		return globalRedisClient
	}

	opts := config.CacheOptions()
	if opts == nil {
		return nil
	}

	globalRedisClient = redis.NewClient(opts)

	return globalRedisClient
}
