package cache

import (
	"testing"
	"time"

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
