package auth_test

import (
	"testing"

	"github.com/cozy/cozy-stack/web/auth"
	"github.com/go-redis/redis"
	"github.com/stretchr/testify/assert"
)

const redisURL = "redis://localhost:6379/0"

func TestLoginRateNotExceededMem(t *testing.T) {
	auth.GlobalCounter = auth.NewMemCounter()
	assert.NoError(t, auth.CheckRateLimit(testInstance, "auth"))
}
func TestLoginRateExceededMem(t *testing.T) {
	auth.GlobalCounter = auth.NewMemCounter()
	for i := 1; i <= 1000; i++ {
		assert.NoError(t, auth.CheckRateLimit(testInstance, "auth"))
	}

	err := auth.CheckRateLimit(testInstance, "auth")
	assert.Error(t, err)
}

func TestLoginRateNotExceededRedis(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	client.Del("auth:" + testInstance.Domain)
	auth.GlobalCounter = &auth.RedisCounter{Client: client}

	assert.NoError(t, auth.CheckRateLimit(testInstance, "auth"))
}
func TestLoginRateExceededRedis(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	auth.GlobalCounter = &auth.RedisCounter{Client: client}
	client.Del("auth:" + testInstance.Domain)

	for i := 1; i <= 1000; i++ {
		assert.NoError(t, auth.CheckRateLimit(testInstance, "auth"))
	}
	assert.Error(t, auth.CheckRateLimit(testInstance, "auth"))
}

func Test2FAGenerationNotExceededMem(t *testing.T) {
	auth.GlobalCounter = auth.NewMemCounter()
	assert.NoError(t, auth.CheckRateLimit(testInstance, "two-factor-generation"))

}
func Test2FAGenerationExceededMem(t *testing.T) {
	auth.GlobalCounter = auth.NewMemCounter()
	for i := 1; i <= 20; i++ {
		assert.NoError(t, auth.CheckRateLimit(testInstance, "two-factor-generation"))
	}

	err := auth.CheckRateLimit(testInstance, "two-factor-generation")
	assert.Error(t, err)
}

func Test2FAGenerationNotExceededRedis(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	client.Del("two-factor-generation:" + testInstance.Domain)
	auth.GlobalCounter = &auth.RedisCounter{Client: client}

	assert.NoError(t, auth.CheckRateLimit(testInstance, "two-factor-generation"))
}
func Test2FAGenerationExceededRedis(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	auth.GlobalCounter = &auth.RedisCounter{Client: client}
	client.Del("two-factor-generation:" + testInstance.Domain)

	for i := 1; i <= 20; i++ {
		assert.NoError(t, auth.CheckRateLimit(testInstance, "two-factor-generation"))
	}
	assert.Error(t, auth.CheckRateLimit(testInstance, "two-factor-generation"))
}

func Test2FARateExceededNotExceededMem(t *testing.T) {
	auth.GlobalCounter = auth.NewMemCounter()
	assert.NoError(t, auth.CheckRateLimit(testInstance, "two-factor"))
}

func Test2FARateExceededMem(t *testing.T) {
	auth.GlobalCounter = auth.NewMemCounter()
	for i := 1; i <= 10; i++ {
		assert.NoError(t, auth.CheckRateLimit(testInstance, "two-factor"))
	}

	err := auth.CheckRateLimit(testInstance, "two-factor")
	assert.Error(t, err)
}
func Test2FANotExceededRedis(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	client.Del("two-factor:" + testInstance.Domain)
	auth.GlobalCounter = &auth.RedisCounter{Client: client}

	assert.NoError(t, auth.CheckRateLimit(testInstance, "two-factor"))
}
func Test2FAExceededRedis(t *testing.T) {
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	auth.GlobalCounter = &auth.RedisCounter{Client: client}
	client.Del("two-factor:" + testInstance.Domain)

	for i := 1; i <= 10; i++ {
		assert.NoError(t, auth.CheckRateLimit(testInstance, "two-factor"))
	}
	assert.Error(t, auth.CheckRateLimit(testInstance, "two-factor"))
}
