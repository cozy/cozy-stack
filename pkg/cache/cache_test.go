package cache

import (
	"testing"
	"time"

	"github.com/go-redis/redis/v7"
	"github.com/stretchr/testify/assert"
)

func TestCacheInMemory(t *testing.T) {
	key := "foo"
	val := []byte("bar")
	c := New(nil)
	c.Set(key, val, 10*time.Millisecond)
	actual, ok := c.Get(key)
	assert.True(t, ok)
	assert.Equal(t, val, actual)
	time.Sleep(11 * time.Millisecond)
	_, ok = c.Get(key)
	assert.False(t, ok)
}

func TestMultiGet(t *testing.T) {
	redisURL := "redis://localhost:6379/0"
	opts, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opts)
	c := New(client)
	c.Set("one", []byte("1"), 10*time.Millisecond)
	c.Set("two", []byte("2"), 10*time.Millisecond)
	bufs := c.MultiGet([]string{"one", "two", "three"})
	assert.Len(t, bufs, 3)
	assert.Equal(t, []byte("1"), bufs[0])
	assert.Equal(t, []byte("2"), bufs[1])
	assert.Nil(t, bufs[2])
}
