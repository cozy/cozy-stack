package logger

import (
	"bytes"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type hookMock struct {
	mock.Mock
}

func newHookMock(t *testing.T) *hookMock {
	m := new(hookMock)
	m.Test(t)

	t.Cleanup(func() { m.AssertExpectations(t) })

	return m
}

func (m *hookMock) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (m *hookMock) Fire(entry *logrus.Entry) error {
	return m.Called(entry.String()).Error(0)
}

func TestLogger(t *testing.T) {
	t.Run("DebugDomain", func(t *testing.T) {
		buf := new(bytes.Buffer)
		err := Init(Options{
			Level:  "info",
			Output: buf,
		})
		assert.NoError(t, err)

		debugLogger.SetFormatter(&logrus.TextFormatter{
			DisableColors:    true,
			DisableTimestamp: true,
		})

		err = AddDebugDomain("foo.bar", 24*time.Hour)
		assert.NoError(t, err)

		WithDomain("foo.bar").Debug("debug1")

		debugTimeout := DebugExpiration("foo.bar")
		assert.WithinDuration(t, time.Now().Add(24*time.Hour), *debugTimeout, time.Second)

		err = RemoveDebugDomain("foo.bar")
		assert.NoError(t, err)

		WithDomain("foo.bar").Debug("debug2")

		assert.Equal(t, "level=debug msg=debug1 domain=foo.bar\n", buf.String())
	})

	t.Run("DebugDomain with expired debug", func(t *testing.T) {
		buf := new(bytes.Buffer)

		err := Init(Options{
			Level:  "info",
			Output: buf,
		})
		assert.NoError(t, err)

		debugLogger.SetFormatter(&logrus.TextFormatter{
			DisableColors:    true,
			DisableTimestamp: true,
		})

		err = AddDebugDomain("foo.bar", 5*time.Millisecond)
		assert.NoError(t, err)

		WithDomain("foo.bar").Debug("debug1")

		// Let the rule about the domain expire.
		time.Sleep(6 * time.Millisecond)

		// Should not be logged as the debug mode has expired
		WithDomain("foo.bar").Debug("debug2")

		debugTimeout := DebugExpiration("foo.bar")
		assert.Nil(t, debugTimeout)

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

	t.Run("Hooks are triggered", func(t *testing.T) {
		hook := newHookMock(t)

		err := Init(Options{
			Level:  "info",
			Hooks:  []logrus.Hook{hook},
			Output: io.Discard,
		})
		require.NoError(t, err)

		logrus.StandardLogger().SetFormatter(&logrus.TextFormatter{
			DisableColors:    true,
			DisableTimestamp: true,
		})

		hook.On("Fire", "level=warning msg=warn1 domain=foo.bar\n", nil).Once().Return(nil)

		WithDomain("foo.bar").Warnf("warn%d", 1)
	})

	t.Run("Fallback to Info if level is not set", func(t *testing.T) {
		err := Init(Options{})
		require.NoError(t, err)

		assert.Equal(t, logrus.InfoLevel, logrus.StandardLogger().Level)
	})

	t.Run("Init fail in case of an invalid level", func(t *testing.T) {
		err := Init(Options{Level: "invalid level"})

		require.EqualError(t, err, "not a valid logrus Level: \"invalid level\"")
	})

	t.Run("Truncate log line if too long", func(t *testing.T) {
		buf := new(bytes.Buffer)

		err := Init(Options{
			Level:  "info",
			Output: buf,
		})
		require.NoError(t, err)

		debugLogger.SetFormatter(&logrus.TextFormatter{
			DisableColors:    true,
			DisableTimestamp: true,
		})

		// input == "foo" + ' ' * 3000 + "bar"
		WithDomain("test").Error(fmt.Sprintf("%-3000sbar", "foo"))

		assert.Equal(t, fmt.Sprintf("level=error msg=\"%-1988s [TRUNCATED]\" domain=test\n", "foo"), buf.String())
	})
}
