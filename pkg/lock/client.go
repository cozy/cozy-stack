package lock

import (
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/go-redis/redis"
)

type subRedisInterface interface {
	SetNX(key string, value interface{}, expiration time.Duration) *redis.BoolCmd
	Eval(script string, keys []string, args ...interface{}) *redis.Cmd
}

var mu sync.Mutex
var globalRedisClient subRedisInterface

func getClient() subRedisInterface {
	mu.Lock()
	defer mu.Unlock()

	if globalRedisClient != nil {
		return globalRedisClient
	}

	opts := config.LockOptions()
	if opts == nil {
		return nil
	}

	globalRedisClient = redis.NewClient(opts)

	return globalRedisClient
}
