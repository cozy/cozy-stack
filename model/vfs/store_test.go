package vfs

import (
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestStore(t *testing.T) {
	config.UseTestFile(t)

	t.Run("StoreInMemory", func(t *testing.T) {
		wasStoreTTL := storeTTL
		storeTTL = 100 * time.Millisecond
		defer func() { storeTTL = wasStoreTTL }()

		dbA := prefixer.NewPrefixer(0, "alice.cozycloud.local", "alice.cozycloud.local")
		dbB := prefixer.NewPrefixer(0, "bob.cozycloud.local", "bob.cozycloud.local")
		store := newMemStore()

		path := "/test/random/path.txt"
		key1, err := store.AddFile(dbA, path)
		assert.NoError(t, err)

		path2, err := store.GetFile(dbB, key1)
		assert.Equal(t, ErrWrongToken, err)
		assert.Zero(t, path2, "Inter-instances store leaking")

		path3, err := store.GetFile(dbA, key1)
		assert.NoError(t, err)
		assert.Equal(t, path, path3)

		time.Sleep(2 * storeTTL)

		path4, err := store.GetFile(dbA, key1)
		assert.Equal(t, ErrWrongToken, err)
		assert.Zero(t, path4, "no expiration")

		a := &Archive{
			Name: "test",
			Files: []string{
				"/archive/foo.jpg",
				"/archive/bar",
			},
		}
		key2, err := store.AddArchive(dbA, a)
		assert.NoError(t, err)

		a2, err := store.GetArchive(dbA, key2)
		assert.NoError(t, err)
		assert.Equal(t, a, a2)

		time.Sleep(2 * storeTTL)

		a3, err := store.GetArchive(dbA, key2)
		assert.Equal(t, ErrWrongToken, err)
		assert.Nil(t, a3, "no expiration")

		m := &Metadata{"foo": "bar"}
		key3, err := store.AddMetadata(dbA, m)
		assert.NoError(t, err)

		m2, err := store.GetMetadata(dbA, key3)
		assert.NoError(t, err)
		assert.Equal(t, m, m2)

		time.Sleep(2 * storeTTL)

		m3, err := store.GetArchive(dbA, key3)
		assert.Equal(t, ErrWrongToken, err)
		assert.Nil(t, m3, "no expiration")
	})

	t.Run("StoreInRedis", func(t *testing.T) {
		if testing.Short() {
			t.Skip("a redis is required for this test: test skipped due to the use of --short flag")
		}

		wasStoreTTL := storeTTL
		storeTTL = 100 * time.Millisecond
		defer func() { storeTTL = wasStoreTTL }()

		dbA := prefixer.NewPrefixer(0, "alice.cozycloud.local", "alice.cozycloud.local")
		dbB := prefixer.NewPrefixer(0, "bob.cozycloud.local", "bob.cozycloud.local")
		opt, err := redis.ParseURL("redis://localhost:6379/15")
		assert.NoError(t, err)
		cli := redis.NewClient(opt)
		store := newRedisStore(cli)

		path := "/test/random/path.txt"
		key1, err := store.AddFile(dbA, path)
		assert.NoError(t, err)

		path2, err := store.GetFile(dbB, key1)
		assert.Equal(t, ErrWrongToken, err)
		assert.Zero(t, path2, "Inter-instances store leaking")

		path3, err := store.GetFile(dbA, key1)
		assert.NoError(t, err)
		assert.Equal(t, path, path3)

		time.Sleep(2 * storeTTL)

		path4, err := store.GetFile(dbA, key1)
		assert.Equal(t, ErrWrongToken, err)
		assert.Zero(t, path4, "no expiration")

		a := &Archive{
			Name: "test",
			Files: []string{
				"/archive/foo.jpg",
				"/archive/bar",
			},
		}
		key2, err := store.AddArchive(dbA, a)
		assert.NoError(t, err)

		a2, err := store.GetArchive(dbA, key2)
		assert.NoError(t, err)
		assert.Equal(t, a, a2)

		time.Sleep(2 * storeTTL)

		a3, err := store.GetArchive(dbA, key2)
		assert.Equal(t, ErrWrongToken, err)
		assert.Nil(t, a3, "no expiration")

		m := &Metadata{"foo": "bar"}
		key3, err := store.AddMetadata(dbA, m)
		assert.NoError(t, err)

		m2, err := store.GetMetadata(dbA, key3)
		assert.NoError(t, err)
		assert.Equal(t, m, m2)

		time.Sleep(2 * storeTTL)

		m3, err := store.GetArchive(dbA, key3)
		assert.Equal(t, ErrWrongToken, err)
		assert.Nil(t, m3, "no expiration")
	})
}
