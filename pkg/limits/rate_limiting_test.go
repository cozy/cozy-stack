package limits

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRate(t *testing.T) {
	var testInstance = prefixer.NewPrefixer(0, "cozy.example.net", "cozy-example-limits")

	rOpt, err := redis.ParseURL("redis://localhost:6379/0")
	require.NoError(t, err)

	redisClient := redis.NewClient(rOpt)

	tests := []struct {
		Name      string
		Client    Counter
		NeedRedis bool
	}{
		{
			Name:      "InMemory",
			Client:    NewInMemory(),
			NeedRedis: false,
		},
		{
			Name:      "Redis",
			Client:    NewRedis(redisClient),
			NeedRedis: true,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			if test.NeedRedis && testing.Short() {
				t.Skip("a redis is required for this test: test skipped due to the use of --short flag")
			}

			limiter := &RateLimiter{counter: test.Client}

			t.Run("LoginRateNotExceeded", func(t *testing.T) {
				require.NoError(t, limiter.CheckRateLimit(testInstance, AuthType))
			})

			t.Run("LoginRateExceeded", func(t *testing.T) {
				// Take into account the call above
				for i := 1; i < 1000; i++ {
					require.NoError(t, limiter.CheckRateLimit(testInstance, AuthType))
				}
				err := limiter.CheckRateLimit(testInstance, AuthType)
				require.Error(t, err)
			})

			t.Run("2FAGenerationNotExceeded", func(t *testing.T) {
				require.NoError(t, limiter.CheckRateLimit(testInstance, TwoFactorGenerationType))
			})

			t.Run("2FAGenerationExceeded", func(t *testing.T) {
				// Take into account the call above
				for i := 1; i < 20; i++ {
					require.NoError(t, limiter.CheckRateLimit(testInstance, TwoFactorGenerationType))
				}

				err := limiter.CheckRateLimit(testInstance, TwoFactorGenerationType)
				require.Error(t, err)
			})

			t.Run("2FARateExceededNotExceeded", func(t *testing.T) {
				require.NoError(t, limiter.CheckRateLimit(testInstance, TwoFactorType))
			})

			t.Run("2FARateExceeded", func(t *testing.T) {
				// Take into account the call above
				for i := 1; i < 10; i++ {
					require.NoError(t, limiter.CheckRateLimit(testInstance, TwoFactorType))
				}

				err := limiter.CheckRateLimit(testInstance, TwoFactorType)
				require.Error(t, err)
			})
		})
	}
}
