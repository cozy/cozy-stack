package cache

import (
	"sort"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func GenericCacheImplementsCache(t *testing.T) {
	require.Implements(t, (*Cache)(nil), new(GenericCache))
}

func TestRedisCache(t *testing.T) {
	redisURL := "redis://localhost:6379/0"
	opts, err := redis.ParseURL(redisURL)
	require.NoError(t, err)

	redisClient := NewRedis(redis.NewClient(opts))
	genericCacheWithClient := New(redis.NewClient(opts))
	genericCacheWithoutClient := New(nil)

	tests := []struct {
		name        string
		client      Cache
		RequireInte bool
	}{
		{
			name:        "GenericCacheWithClient",
			client:      &genericCacheWithClient,
			RequireInte: true,
		},
		{
			name:        "RedisClient",
			client:      redisClient,
			RequireInte: true,
		},
		{
			name:        "GenericCacheWithoutClient",
			client:      &genericCacheWithoutClient,
			RequireInte: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if testing.Short() && test.RequireInte {
				t.Skip("a redis is required for this test: test skipped due to the use of --short flag")
			}

			c := test.client

			t.Run("SET/GET/EXPIRE", func(t *testing.T) {
				key := "foo"
				val := []byte("bar")

				// Set a key/value pair with a 10 ms expiration
				c.Set(key, val, 10*time.Millisecond)

				// Fetch the pair
				actual, ok := c.Get(key)
				assert.True(t, ok)
				assert.Equal(t, val, actual)

				// Wait the pair expiration
				time.Sleep(11 * time.Millisecond)

				// Check that the pair is no more available
				_, ok = c.Get(key)
				assert.False(t, ok)
			})

			t.Run("Keys", func(t *testing.T) {
				c.Set("foo:one", []byte("1"), 10*time.Millisecond)
				c.Set("foo:two", []byte("2"), 10*time.Millisecond)
				c.Set("bar:baz", []byte("3"), 10*time.Millisecond)
				keys := c.Keys("foo:")
				sort.Strings(keys)
				assert.Equal(t, []string{"foo:one", "foo:two"}, keys)
			})

			t.Run("MultiGet", func(t *testing.T) {
				// Set two values
				c.Set("one", []byte("1"), 10*time.Millisecond)
				c.Set("two", []byte("2"), 10*time.Millisecond)

				// Get the two
				bufs := c.MultiGet([]string{"one", "two", "three"})

				assert.Len(t, bufs, 3)
				assert.Equal(t, []byte("1"), bufs[0])
				assert.Equal(t, []byte("2"), bufs[1])
				assert.Nil(t, bufs[2])
			})

			t.Run("Keys", func(t *testing.T) {
				// Set three values
				c.Set("foo:one", []byte("1"), 10*time.Millisecond)
				c.Set("foo:two", []byte("2"), 10*time.Millisecond)
				c.Set("bar:baz", []byte("3"), 10*time.Millisecond)

				keys := c.Keys("foo:")
				sort.Strings(keys)
				assert.Equal(t, []string{"foo:one", "foo:two"}, keys)
			})
		})
	}
}
