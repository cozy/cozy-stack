package logger

import (
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogger_RedisDebugger(t *testing.T) {
	if testing.Short() {
		t.Skip("a redis is required for this test: test skipped due to the use of --short flag")
	}

	opt, err := redis.ParseURL("redis://localhost:6379/0")
	require.NoError(t, err)

	t.Run("Success simple instance", func(t *testing.T) {
		dbg, err := NewRedisDebugger(redis.NewClient(opt))
		require.NoError(t, err)
		defer dbg.Close()

		domain := createValidName(t)

		err = dbg.AddDomain(domain, time.Second)
		require.NoError(t, err)

		time.Sleep(30 * time.Millisecond)

		expirationDate := dbg.ExpiresAt(domain)
		assert.NotNil(t, expirationDate)
	})

	t.Run("a new domain is propagated across instances", func(t *testing.T) {
		dbg1, err := NewRedisDebugger(redis.NewClient(opt))
		require.NoError(t, err)
		defer dbg1.Close()

		dbg2, err := NewRedisDebugger(redis.NewClient(opt))
		require.NoError(t, err)
		defer dbg2.Close()

		domain := createValidName(t)

		// The instance "dbg1" add the domain "foo"
		err = dbg1.AddDomain(domain, time.Second)
		require.NoError(t, err)

		time.Sleep(30 * time.Millisecond)

		// The instance "dbg2" also have the expiration
		expirationDate := dbg2.ExpiresAt(domain)
		assert.NotNil(t, expirationDate)

		// Delete the key with "dbg2"
		dbg2.RemoveDomain(domain)

		time.Sleep(30 * time.Millisecond)

		// The key doesn't exist for "dbg1" anymore
		expirationDate = dbg1.ExpiresAt(domain)
		assert.Nil(t, expirationDate)
	})

	t.Run("Invalid domain name format", func(t *testing.T) {
		dbg, err := NewRedisDebugger(redis.NewClient(opt))
		require.NoError(t, err)

		err = dbg.AddDomain("foo/bar", time.Second)
		assert.ErrorIs(t, err, ErrInvalidDomainFormat)
	})

	t.Run("Start with an invalid redis client", func(t *testing.T) {
		badOpt, err := redis.ParseURL("redis://invalid:6379/0")
		require.NoError(t, err)

		dbg, err := NewRedisDebugger(redis.NewClient(badOpt))
		assert.Nil(t, dbg)
		require.ErrorContains(t, err, "bootstrap failed")
	})
}

func createValidName(t *testing.T) string {
	return strings.ReplaceAll(t.Name(), "/", "_")
}
