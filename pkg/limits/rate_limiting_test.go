package limits

import (
	"context"
	"testing"

	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

const redisURL = "redis://localhost:6379/0"

var testInstance = prefixer.NewPrefixer(0, "cozy.example.net", "cozy-example-net")

func TestRate(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	t.Run("LoginRateNotExceededMem", func(t *testing.T) {
		globalCounter = NewInMemory()
		assert.NoError(t, CheckRateLimit(testInstance, AuthType))
	})

	t.Run("LoginRateExceededMem", func(t *testing.T) {
		globalCounter = NewInMemory()
		for i := 1; i <= 1000; i++ {
			assert.NoError(t, CheckRateLimit(testInstance, AuthType))
		}
		err := CheckRateLimit(testInstance, AuthType)
		assert.Error(t, err)
	})

	t.Run("LoginRateNotExceededRedis", func(t *testing.T) {
		opts, _ := redis.ParseURL(redisURL)
		client := redis.NewClient(opts)
		client.Del(context.Background(), "auth:"+testInstance.DomainName())
		globalCounter = NewRedisCounter(client)
		assert.NoError(t, CheckRateLimit(testInstance, AuthType))
	})

	t.Run("LoginRateExceededRedis", func(t *testing.T) {
		opts, _ := redis.ParseURL(redisURL)
		client := redis.NewClient(opts)
		globalCounter = NewRedisCounter(client)
		client.Del(context.Background(), "auth:"+testInstance.DomainName())
		for i := 1; i <= 1000; i++ {
			assert.NoError(t, CheckRateLimit(testInstance, AuthType))
		}
		assert.Error(t, CheckRateLimit(testInstance, AuthType))
	})

	t.Run("2FAGenerationNotExceededMem", func(t *testing.T) {
		globalCounter = NewInMemory()
		assert.NoError(t, CheckRateLimit(testInstance, TwoFactorGenerationType))
	})

	t.Run("2FAGenerationExceededMem", func(t *testing.T) {
		globalCounter = NewInMemory()
		for i := 1; i <= 20; i++ {
			assert.NoError(t, CheckRateLimit(testInstance, TwoFactorGenerationType))
		}
		err := CheckRateLimit(testInstance, TwoFactorGenerationType)
		assert.Error(t, err)
	})

	t.Run("2FAGenerationNotExceededRedis", func(t *testing.T) {
		opts, _ := redis.ParseURL(redisURL)
		client := redis.NewClient(opts)
		client.Del(context.Background(), "two-factor-generation:"+testInstance.DomainName())
		globalCounter = NewRedisCounter(client)
		assert.NoError(t, CheckRateLimit(testInstance, TwoFactorGenerationType))
	})

	t.Run("2FAGenerationExceededRedis", func(t *testing.T) {
		opts, _ := redis.ParseURL(redisURL)
		client := redis.NewClient(opts)
		globalCounter = NewRedisCounter(client)
		client.Del(context.Background(), "two-factor-generation:"+testInstance.DomainName())
		for i := 1; i <= 20; i++ {
			assert.NoError(t, CheckRateLimit(testInstance, TwoFactorGenerationType))
		}
		assert.Error(t, CheckRateLimit(testInstance, TwoFactorGenerationType))
	})

	t.Run("2FARateExceededNotExceededMem", func(t *testing.T) {
		globalCounter = NewInMemory()
		assert.NoError(t, CheckRateLimit(testInstance, TwoFactorType))
	})

	t.Run("2FARateExceededMem", func(t *testing.T) {
		globalCounter = NewInMemory()
		for i := 1; i <= 10; i++ {
			assert.NoError(t, CheckRateLimit(testInstance, TwoFactorType))
		}
		err := CheckRateLimit(testInstance, TwoFactorType)
		assert.Error(t, err)
	})

	t.Run("2FANotExceededRedis", func(t *testing.T) {
		opts, _ := redis.ParseURL(redisURL)
		client := redis.NewClient(opts)
		client.Del(context.Background(), "two-factor:"+testInstance.DomainName())
		globalCounter = NewRedisCounter(client)
		assert.NoError(t, CheckRateLimit(testInstance, TwoFactorType))
	})

	t.Run("2FAExceededRedis", func(t *testing.T) {
		opts, _ := redis.ParseURL(redisURL)
		client := redis.NewClient(opts)
		globalCounter = NewRedisCounter(client)
		client.Del(context.Background(), "two-factor:"+testInstance.DomainName())
		for i := 1; i <= 10; i++ {
			assert.NoError(t, CheckRateLimit(testInstance, TwoFactorType))
		}
		assert.Error(t, CheckRateLimit(testInstance, TwoFactorType))
	})
}
