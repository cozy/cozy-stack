package limits

import (
	"errors"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/go-redis/redis"
)

// Counter is an interface for counting number of attempts that can be used to
// rate limit the number of logins and 2FA tries, and thus block bruteforce
// attacks.
type Counter interface {
	Increment(key string, timeLimit time.Duration) (int64, error)
	Reset(key string) error
}

var globalCounter Counter
var globalCounterMu sync.Mutex
var counterCleanInterval = 1 * time.Second

func getCounter() Counter {
	globalCounterMu.Lock()
	defer globalCounterMu.Unlock()
	if globalCounter != nil {
		return globalCounter
	}
	client := config.GetConfig().RateLimitingStorage.Client()
	if client == nil {
		globalCounter = NewMemCounter()
	} else {
		globalCounter = NewRedisCounter(client)
	}
	return globalCounter
}

type memRef struct {
	val int64
	exp time.Time
}
type memCounter struct {
	mu   sync.Mutex
	vals map[string]*memRef
}

// NewMemCounter returns a in-memory counter.
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

func (c *memCounter) Reset(key string) error {
	delete(c.vals, key)
	return nil
}

type redisCounter struct {
	Client redis.UniversalClient
}

// NewRedisCounter returns a counter that can be mutualized between several
// cozy-stack processes by using redis.
func NewRedisCounter(client redis.UniversalClient) Counter {
	return &redisCounter{client}
}

func (r *redisCounter) Increment(key string, timeLimit time.Duration) (int64, error) {
	count, err := r.Client.Incr(key).Result()
	if err != nil {
		return 0, err
	}
	if count == 1 {
		r.Client.Expire(key, timeLimit)
	}
	return count, nil
}

func (r *redisCounter) Reset(key string) error {
	_, err := r.Client.Del(key).Result()
	return err
}

// CheckRateLimit returns an error if the counter for the given type and
// instance has reached the limit.
func CheckRateLimit(inst *instance.Instance, passwordType string) error {
	const MaxAuthTries = 1000
	const MaxAuthGenerations = 20
	const Max2FATries = 10

	var key string
	var limit int64
	var timeLimit time.Duration
	switch passwordType {
	case "auth":
		key = "auth:" + inst.Domain
		limit = MaxAuthTries
		timeLimit = 3600 * time.Second // 1 hour
	case "two-factor-generation":
		key = "two-factor-generation:" + inst.Domain
		limit = MaxAuthGenerations
		timeLimit = 3600 * time.Second // 1 hour
	case "two-factor":
		key = "two-factor:" + inst.Domain
		limit = Max2FATries
		timeLimit = 300 * time.Second // 5 minutes
	}

	counter := getCounter()
	val, err := counter.Increment(key, timeLimit)
	if err != nil {
		return err
	}
	if val <= limit {
		return nil
	}
	return errors.New("Rate limit exceeded")
}

// ResetCounter sets again to zero the counter for the given type and instance.
func ResetCounter(inst *instance.Instance, passwordType string) {
	counter := getCounter()
	_ = counter.Reset(passwordType + ":" + inst.Domain)
}
