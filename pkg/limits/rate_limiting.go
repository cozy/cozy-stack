package limits

import (
	"errors"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/go-redis/redis"
)

// CounterType os an enum for the type of counters used by rate-limiting.
type CounterType int

const (
	// AuthType is used for counting the number of login attempts.
	AuthType CounterType = iota
	// TwoFactorGenerationType is used for counting the number of times a 2FA
	// is generated.
	TwoFactorGenerationType
	// TwoFactorType is used for counting the number of 2FA attempts.
	TwoFactorType
	// OAuthClientType is used for counting the number of OAuth clients.
	// creations/updates.
	OAuthClientType
	// SharingInviteType is used for counting the number of sharing invitations
	// sent to a given instance.
	SharingInviteType
	// SharingPublicLinkType is used for counting the number of public sharing
	// link consultations
	SharingPublicLinkType
)

type counterConfig struct {
	Prefix string
	Limit  int64
	Period time.Duration
}

var configs = []counterConfig{
	// AuthType
	{
		Prefix: "auth",
		Limit:  1000,
		Period: 1 * time.Hour,
	},
	// TwoFactorGenerationType
	{
		Prefix: "two-factor-generation",
		Limit:  20,
		Period: 1 * time.Hour,
	},
	// TwoFactorType
	{
		Prefix: "two-factor",
		Limit:  10,
		Period: 5 * time.Minute,
	},
	// OAuthClientType
	{
		Prefix: "oauth-client",
		Limit:  20,
		Period: 1 * time.Hour,
	},
	// SharingInviteType
	{
		Prefix: "sharing-invite",
		Limit:  10,
		Period: 1 * time.Hour,
	},
	// SharingPublicLink
	{
		Prefix: "sharing-public-link",
		Limit:  2000,
		Period: 1 * time.Hour,
	},
}

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
func CheckRateLimit(p prefixer.Prefixer, ct CounterType) error {
	return CheckRateLimitKey(p.DomainName(), ct)
}

// CheckRateLimitKey allows to check the rate-limit for a key
func CheckRateLimitKey(customKey string, ct CounterType) error {
	cfg := configs[ct]
	key := cfg.Prefix + ":" + customKey
	val, err := getCounter().Increment(key, cfg.Period)
	if err != nil {
		return err
	}
	if val > cfg.Limit {
		return errors.New("Rate limit exceeded")
	}
	return nil
}

// ResetCounter sets again to zero the counter for the given type and instance.
func ResetCounter(p prefixer.Prefixer, ct CounterType) {
	cfg := configs[ct]
	key := cfg.Prefix + ":" + p.DomainName()
	_ = getCounter().Reset(key)
}
