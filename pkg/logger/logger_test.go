package logger

import (
	"bytes"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestLogger(t *testing.T) {
	t.Run("DebugDomain", func(t *testing.T) {
		err := Init(Options{Level: "info"})
		assert.NoError(t, err)

		buf := new(bytes.Buffer)
		debugLogger.SetFormatter(&logrus.TextFormatter{
			DisableColors:    true,
			DisableTimestamp: true,
		})
		debugLogger.SetOutput(buf)

		err = AddDebugDomain("foo.bar", 24*time.Hour)
		assert.NoError(t, err)
		err = AddDebugDomain("foo.bar", 24*time.Hour)
		assert.NoError(t, err)

		WithDomain("foo.bar").Debug("debug1")

		err = RemoveDebugDomain("foo.bar")
		assert.NoError(t, err)

		WithDomain("foo.bar").Debug("debug2")

		assert.Equal(t, "level=debug msg=debug1 domain=foo.bar\n", buf.String())
	})

	t.Run("DebugDomainWithRedis", func(t *testing.T) {
		if testing.Short() {
			t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
		}

		opt, err := redis.ParseURL("redis://localhost:6379/0")
		assert.NoError(t, err)
		err = Init(Options{
			Level: "info",
			Redis: redis.NewClient(opt),
		})
		assert.NoError(t, err)

		buf := new(bytes.Buffer)
		debugLogger.SetFormatter(&logrus.TextFormatter{
			DisableColors:    true,
			DisableTimestamp: true,
		})
		debugLogger.SetOutput(buf)

		time.Sleep(100 * time.Millisecond)

		err = AddDebugDomain("foo.bar.redis", 24*time.Hour)
		assert.NoError(t, err)
		err = AddDebugDomain("foo.bar.redis", 24*time.Hour)
		assert.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		WithDomain("foo.bar.redis").Debug("debug1")

		err = RemoveDebugDomain("foo.bar.redis")
		assert.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		WithDomain("foo.bar.redis").Debug("debug2")

		assert.Equal(t, "level=debug msg=debug1 domain=foo.bar.redis\n", buf.String())
	})
}
