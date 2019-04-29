package limits

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/go-redis/redis"
	"github.com/stretchr/testify/assert"
)

const redisURL = "redis://localhost:6379/0"

var testInstance = &instance.Instance{Domain: "cozy.example.net"}

func TestLoginRateNotExceededMem(t *testing.T) {
	globalCounter = NewMemCounter()
	assert.NoError(t, CheckRateLimit(testInstance, "auth"))
}

func TestLoginRateExceededMem(t *testing.T) {
	globalCounter = NewMemCounter()
	for i := 1; i <= 1000; i++ {
		assert.NoError(t, CheckRateLimit(testInstance, "auth"))
	}
	err := CheckRateLimit(testInstance, "auth")
	assert.Error(t, err)
}

func TestLoginRateNotExceededRedis(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	client.Del("auth:" + testInstance.Domain)
	globalCounter = NewRedisCounter(client)
	assert.NoError(t, CheckRateLimit(testInstance, "auth"))
}

func TestLoginRateExceededRedis(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	globalCounter = NewRedisCounter(client)
	client.Del("auth:" + testInstance.Domain)
	for i := 1; i <= 1000; i++ {
		assert.NoError(t, CheckRateLimit(testInstance, "auth"))
	}
	assert.Error(t, CheckRateLimit(testInstance, "auth"))
}

func Test2FAGenerationNotExceededMem(t *testing.T) {
	globalCounter = NewMemCounter()
	assert.NoError(t, CheckRateLimit(testInstance, "two-factor-generation"))
}

func Test2FAGenerationExceededMem(t *testing.T) {
	globalCounter = NewMemCounter()
	for i := 1; i <= 20; i++ {
		assert.NoError(t, CheckRateLimit(testInstance, "two-factor-generation"))
	}
	err := CheckRateLimit(testInstance, "two-factor-generation")
	assert.Error(t, err)
}

func Test2FAGenerationNotExceededRedis(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	client.Del("two-factor-generation:" + testInstance.Domain)
	globalCounter = NewRedisCounter(client)
	assert.NoError(t, CheckRateLimit(testInstance, "two-factor-generation"))
}

func Test2FAGenerationExceededRedis(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	globalCounter = NewRedisCounter(client)
	client.Del("two-factor-generation:" + testInstance.Domain)
	for i := 1; i <= 20; i++ {
		assert.NoError(t, CheckRateLimit(testInstance, "two-factor-generation"))
	}
	assert.Error(t, CheckRateLimit(testInstance, "two-factor-generation"))
}

func Test2FARateExceededNotExceededMem(t *testing.T) {
	globalCounter = NewMemCounter()
	assert.NoError(t, CheckRateLimit(testInstance, "two-factor"))
}

func Test2FARateExceededMem(t *testing.T) {
	globalCounter = NewMemCounter()
	for i := 1; i <= 10; i++ {
		assert.NoError(t, CheckRateLimit(testInstance, "two-factor"))
	}
	err := CheckRateLimit(testInstance, "two-factor")
	assert.Error(t, err)
}

func Test2FANotExceededRedis(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	client.Del("two-factor:" + testInstance.Domain)
	globalCounter = NewRedisCounter(client)
	assert.NoError(t, CheckRateLimit(testInstance, "two-factor"))
}

func Test2FAExceededRedis(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	globalCounter = NewRedisCounter(client)
	client.Del("two-factor:" + testInstance.Domain)
	for i := 1; i <= 10; i++ {
		assert.NoError(t, CheckRateLimit(testInstance, "two-factor"))
	}
	assert.Error(t, CheckRateLimit(testInstance, "two-factor"))
}
