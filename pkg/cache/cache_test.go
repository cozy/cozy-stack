package cache

import (
	"io/ioutil"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCacheInMemory(t *testing.T) {
	key := "foo"
	val := []byte("bar")
	c := New(nil)
	c.Set(key, val, 10*time.Millisecond)
	reader, ok := c.Get(key)
	assert.True(t, ok)
	actual, err := ioutil.ReadAll(reader)
	assert.NoError(t, err)
	assert.Equal(t, val, actual)
	time.Sleep(11 * time.Millisecond)
	_, ok = c.Get(key)
	assert.False(t, ok)
}
