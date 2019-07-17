package limits

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/go-redis/redis"
	"github.com/stretchr/testify/assert"
)

const redisURL = "redis://localhost:6379/0"

var testInstance = prefixer.NewPrefixer("cozy.example.net", "cozy-example-net")

func TestLoginRateNotExceededMem(t *testing.T) {
	globalCounter = NewMemCounter()
	assert.NoError(t, CheckRateLimit(testInstance, AuthType))
}

func TestLoginRateExceededMem(t *testing.T) {
	globalCounter = NewMemCounter()
	for i := 1; i <= 1000; i++ {
		assert.NoError(t, CheckRateLimit(testInstance, AuthType))
	}
	err := CheckRateLimit(testInstance, AuthType)
	assert.Error(t, err)
}

func TestLoginRateNotExceededRedis(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	client.Del("auth:" + testInstance.DomainName())
	globalCounter = NewRedisCounter(client)
	assert.NoError(t, CheckRateLimit(testInstance, AuthType))
}

func TestLoginRateExceededRedis(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	globalCounter = NewRedisCounter(client)
	client.Del("auth:" + testInstance.DomainName())
	for i := 1; i <= 1000; i++ {
		assert.NoError(t, CheckRateLimit(testInstance, AuthType))
	}
	assert.Error(t, CheckRateLimit(testInstance, AuthType))
}

func Test2FAGenerationNotExceededMem(t *testing.T) {
	globalCounter = NewMemCounter()
	assert.NoError(t, CheckRateLimit(testInstance, TwoFactorGenerationType))
}

func Test2FAGenerationExceededMem(t *testing.T) {
	globalCounter = NewMemCounter()
	for i := 1; i <= 20; i++ {
		assert.NoError(t, CheckRateLimit(testInstance, TwoFactorGenerationType))
	}
	err := CheckRateLimit(testInstance, TwoFactorGenerationType)
	assert.Error(t, err)
}

func Test2FAGenerationNotExceededRedis(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	client.Del("two-factor-generation:" + testInstance.DomainName())
	globalCounter = NewRedisCounter(client)
	assert.NoError(t, CheckRateLimit(testInstance, TwoFactorGenerationType))
}

func Test2FAGenerationExceededRedis(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	globalCounter = NewRedisCounter(client)
	client.Del("two-factor-generation:" + testInstance.DomainName())
	for i := 1; i <= 20; i++ {
		assert.NoError(t, CheckRateLimit(testInstance, TwoFactorGenerationType))
	}
	assert.Error(t, CheckRateLimit(testInstance, TwoFactorGenerationType))
}

func Test2FARateExceededNotExceededMem(t *testing.T) {
	globalCounter = NewMemCounter()
	assert.NoError(t, CheckRateLimit(testInstance, TwoFactorType))
}

func Test2FARateExceededMem(t *testing.T) {
	globalCounter = NewMemCounter()
	for i := 1; i <= 10; i++ {
		assert.NoError(t, CheckRateLimit(testInstance, TwoFactorType))
	}
	err := CheckRateLimit(testInstance, TwoFactorType)
	assert.Error(t, err)
}

func Test2FANotExceededRedis(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	client.Del("two-factor:" + testInstance.DomainName())
	globalCounter = NewRedisCounter(client)
	assert.NoError(t, CheckRateLimit(testInstance, TwoFactorType))
}

func Test2FAExceededRedis(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	globalCounter = NewRedisCounter(client)
	client.Del("two-factor:" + testInstance.DomainName())
	for i := 1; i <= 10; i++ {
		assert.NoError(t, CheckRateLimit(testInstance, TwoFactorType))
	}
	assert.Error(t, CheckRateLimit(testInstance, TwoFactorType))
}
