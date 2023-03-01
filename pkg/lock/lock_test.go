// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// GOMAXPROCS=10 go test

package lock

import (
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLock(t *testing.T) {
	redisURL := "redis://localhost:6379/0"
	opts, perr := redis.ParseURL(redisURL)
	require.NoError(t, perr)

	tests := []struct {
		name         string
		client       Getter
		RequireRedis bool
	}{
		{
			name:         "RedisClient",
			client:       New(redis.NewClient(opts)),
			RequireRedis: true,
		},
		{
			name:         "InMemoryClient",
			client:       New(nil),
			RequireRedis: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := test.client
			db := prefixer.NewPrefixer(0, "cozy.local", "cozy.local-lock")

			if testing.Short() && test.RequireRedis {
				t.Skip("a redis is required for this test: test skipped due to the use of --short flag")
			}

			t.Run("SimpleLock", func(t *testing.T) {
				// Lock the resource
				lock := c.ReadWrite(db, t.Name())

				err := lock.Lock()
				require.NoError(t, err)

				lock.Unlock()
			})

			t.Run("LockARessourceTwice", func(t *testing.T) {
				// Create two locks for the same resource
				lock := c.ReadWrite(db, t.Name())
				lock2 := c.ReadWrite(db, t.Name())

				// Lock the resource with the first lock
				err := lock.Lock()
				require.NoError(t, err)

				lock2OK := false
				go func() {
					// Try to lock the resource with lock2, it should wait
					err = lock2.Lock()
					require.NoError(t, err)
					lock2OK = true
				}()

				// Check that the second lock have not locked
				time.Sleep(10 * time.Millisecond)
				assert.False(t, lock2OK)

				// Unlock
				lock.Unlock()

				// Now the resource is free so the lock2 should take the lock.
				time.Sleep(110 * time.Millisecond)
				assert.True(t, lock2OK)

				lock2.Unlock()
			})

			t.Run("LockTwiceWithSameLock", func(t *testing.T) {
				lock := c.ReadWrite(db, t.Name())

				err := lock.Lock()
				require.NoError(t, err)

				// Unlock
				lock.Unlock()

				// Try to read lock the resource, it should wait
				err = lock.RLock()
				require.NoError(t, err)

				lock.RUnlock()
			})

			t.Run("SeveralRLockThenLock", func(t *testing.T) {
				lock := c.ReadWrite(db, t.Name())
				lock2 := c.ReadWrite(db, t.Name())

				// Lock 1
				require.NoError(t, lock.RLock())
				// Lock 2
				require.NoError(t, lock.RLock())
				// Lock 3
				require.NoError(t, lock2.RLock())

				lock2OK := false
				go func() {
					// Try to lock the already locked resource
					err := lock.Lock()
					require.NoError(t, err)
					lock2OK = true
				}()

				// Check that the second lock have not locked
				time.Sleep(10 * time.Millisecond)
				assert.False(t, lock2OK)

				// Unlock all the read locks
				lock.RUnlock()
				lock.RUnlock()
				lock2.RUnlock()

				// Now the resource is free so the lock2 should take the lock.
				time.Sleep(110 * time.Millisecond)
				assert.True(t, lock2OK)

				lock.Unlock()
			})
		})
	}
}
