package cache

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheImplementations(t *testing.T) {
	t.Run("in memory", func(t *testing.T) {
		// In-memory is setup when New is called with nil.
		c := New(nil)

		runTests(t, c)
	})

	t.Run("redis", func(t *testing.T) {
		if testing.Short() {
			t.Skip("a redis is required for this test: test skipped due to the use of --short flag")
		}

		opts, err := redis.ParseURL("redis://localhost:6379/0")
		require.NoError(t, err)

		client := redis.NewClient(opts)

		res := client.Ping(context.Background())
		require.NoError(t, res.Err())

		c := New(client)

		runTests(t, c)
	})
}

func runTests(t *testing.T, c Cache) {
	t.Run("Get", func(t *testing.T) {
		key := "foo"
		val := []byte("bar")
		c.Set(key, val, 10*time.Millisecond)
		actual, ok := c.Get(key)
		assert.True(t, ok)
		assert.Equal(t, val, actual)
		time.Sleep(11 * time.Millisecond)
		_, ok = c.Get(key)
		assert.False(t, ok)
	})

	t.Run("keys", func(t *testing.T) {
		c.Set("foo:one", []byte("1"), 10*time.Millisecond)
		c.Set("foo:two", []byte("2"), 10*time.Millisecond)
		c.Set("bar:baz", []byte("3"), 10*time.Millisecond)
		keys := c.Keys("foo:")
		sort.Strings(keys)
		assert.Equal(t, []string{"foo:one", "foo:two"}, keys)
	})

	t.Run("multi get", func(t *testing.T) {
		c.Set("one", []byte("1"), 10*time.Millisecond)
		c.Set("two", []byte("2"), 10*time.Millisecond)
		bufs := c.MultiGet([]string{"one", "two", "three"})
		assert.Len(t, bufs, 3)
		assert.Equal(t, []byte("1"), bufs[0])
		assert.Equal(t, []byte("2"), bufs[1])
		assert.Nil(t, bufs[2])
	})
}
