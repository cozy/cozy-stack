package auth

import (
	"errors"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/go-redis/redis"
)

type Counter interface {
	Increment(key string, timeLimit time.Duration) (int64, error)
}

var GlobalCounter Counter
var globalCounterMu sync.Mutex
var counterCleanInterval = 1 * time.Second

type memRef struct {
	val int64
	exp time.Time
}
type memCounter struct {
	mu   sync.Mutex
	vals map[string]*memRef
}

type RedisCounter struct {
	Client redis.UniversalClient
}

// GetCounter returns the Counter.
func GetCounter() Counter {
	globalCounterMu.Lock()
	defer globalCounterMu.Unlock()
	if GlobalCounter != nil {
		return GlobalCounter
	}
	client := config.GetConfig().DownloadStorage.Client()
	if client == nil {
		GlobalCounter = NewMemCounter()
	} else {
		GlobalCounter = &RedisCounter{client}
	}
	return GlobalCounter
}

func NewMemCounter() Counter {
	counter := &memCounter{vals: make(map[string]*memRef)}
	go counter.cleaner()
	return counter
}

func (c *memCounter) cleaner() {
	for range time.Tick(counterCleanInterval) {
		now := time.Now()
		for k, v := range c.vals {
			if now.After(v.exp) {
				delete(c.vals, k)
			}
		}
	}
}

func (c *memCounter) Increment(key string, timeLimit time.Duration) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.vals[key]; !ok {
		c.vals[key] = &memRef{
			val: 0,
			exp: time.Now().Add(timeLimit),
		}
	}
	c.vals[key].val++
	return c.vals[key].val, nil
}

func (r *RedisCounter) Increment(key string, timeLimit time.Duration) (int64, error) {
	return r.Client.Incr(key).Result()
}

// CheckRateLimit takes care of bruteforcing
func CheckRateLimit(inst *instance.Instance, passwordType string) error {
	const MaxAuthTries = 1000
	const MaxAuthGenerations = 20
	const Max2FATries = 10

	var key string
	var limit int64
	var timeLimit time.Duration

	counter := GetCounter()

	switch passwordType {
	case "two-factor":
		key = "two-factor:" + inst.Domain
		limit = Max2FATries
		timeLimit = 300 * time.Second // 5 minutes
	case "auth":
		key = "auth:" + inst.Domain
		limit = MaxAuthTries
		timeLimit = 3600 * time.Second // 1 hour
	case "two-factor-generation":
		key = "two-factor-generation:" + inst.Domain
		limit = MaxAuthGenerations
		timeLimit = 3600 * time.Second // 1 hour
	}

	val, err := counter.Increment(key, timeLimit)
	if err != nil {
		return err
	}

	if val <= limit {
		return nil
	}

	return errors.New("Rate limit exceeded")
}

// LoginRateExceeded blocks the instance after too many failed attempts to
// login
func LoginRateExceeded(i *instance.Instance) error {
	t := true
	return instance.Patch(i, &instance.Options{
		Blocked: &t,
	})
}
