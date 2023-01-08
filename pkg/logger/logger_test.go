package logger

import (
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestLogger(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	t.Run("DebugDomain", func(t *testing.T) {
		err := Init(Options{Level: "info"})
		assert.NoError(t, err)

		err = AddDebugDomain("foo.bar", 24*time.Hour)
		assert.NoError(t, err)
		err = AddDebugDomain("foo.bar", 24*time.Hour)
		assert.NoError(t, err)

		log := WithDomain("foo.bar")
		assert.Equal(t, logrus.DebugLevel, log.entry.Logger.Level)

		err = RemoveDebugDomain("foo.bar")
		assert.NoError(t, err)
		log = WithDomain("foo.bar")
		assert.Equal(t, logrus.InfoLevel, log.entry.Logger.Level)
	})

	t.Run("DebugDomainWithRedis", func(t *testing.T) {
		opt, err := redis.ParseURL("redis://localhost:6379/0")
		assert.NoError(t, err)
		err = Init(Options{
			Level: "info",
			Redis: redis.NewClient(opt),
		})
		assert.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		err = AddDebugDomain("foo.bar.redis", 24*time.Hour)
		assert.NoError(t, err)
		err = AddDebugDomain("foo.bar.redis", 24*time.Hour)
		assert.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		log := WithDomain("foo.bar.redis")
		assert.Equal(t, logrus.DebugLevel, log.entry.Logger.Level)

		err = RemoveDebugDomain("foo.bar.redis")
		assert.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		log = WithDomain("foo.bar.redis")
		assert.Equal(t, logrus.InfoLevel, log.entry.Logger.Level)
	})

}
