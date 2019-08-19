package logger

import (
	"testing"
	"time"

	"github.com/go-redis/redis/v7"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestDebugDomain(t *testing.T) {
	err := Init(Options{Level: "info"})
	assert.NoError(t, err)

	err = AddDebugDomain("foo.bar", 24*time.Hour)
	assert.NoError(t, err)
	err = AddDebugDomain("foo.bar", 24*time.Hour)
	assert.NoError(t, err)

	log := WithDomain("foo.bar")
	assert.Equal(t, logrus.DebugLevel, log.Logger.Level)

	err = RemoveDebugDomain("foo.bar")
	assert.NoError(t, err)
	log = WithDomain("foo.bar")
	assert.Equal(t, logrus.InfoLevel, log.Logger.Level)
}

func TestDebugDomainWithRedis(t *testing.T) {
	opt, err := redis.ParseURL("redis://localhost:6379/0")
	assert.NoError(t, err)
	err = Init(Options{
		Level: "info",
		Redis: redis.NewClient(opt),
	})
	assert.NoError(t, err)

	time.Sleep(1 * time.Second)

	err = AddDebugDomain("foo.bar.redis", 24*time.Hour)
	assert.NoError(t, err)
	err = AddDebugDomain("foo.bar.redis", 24*time.Hour)
	assert.NoError(t, err)

	time.Sleep(1 * time.Second)

	log := WithDomain("foo.bar.redis")
	assert.Equal(t, logrus.DebugLevel, log.Logger.Level)

	err = RemoveDebugDomain("foo.bar.redis")
	assert.NoError(t, err)

	time.Sleep(1 * time.Second)

	log = WithDomain("foo.bar.redis")
	assert.Equal(t, logrus.InfoLevel, log.Logger.Level)
}
