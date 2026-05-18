package utils

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryWithBackoffSucceedsAfterRetry(t *testing.T) {
	errTemporary := errors.New("temporary")
	calls := 0

	err := RetryWithBackoff(context.Background(), RetryOptions{
		Attempts: 3,
	}, func() error {
		calls++
		if calls < 3 {
			return errTemporary
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestRetryWithBackoffValueReturnsSuccessfulValue(t *testing.T) {
	errTemporary := errors.New("temporary")
	calls := 0

	value, err := RetryWithBackoffValue(context.Background(), RetryOptions{
		Attempts: 3,
	}, func() (string, error) {
		calls++
		if calls < 2 {
			return "", errTemporary
		}
		return "done", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "done", value)
	assert.Equal(t, 2, calls)
}

func TestRetryWithBackoffStopsOnNonRetryableError(t *testing.T) {
	errRetryable := errors.New("retryable")
	errFatal := errors.New("fatal")
	calls := 0

	err := RetryWithBackoff(context.Background(), RetryOptions{
		Attempts: 5,
		ShouldRetry: func(err error) bool {
			return errors.Is(err, errRetryable)
		},
	}, func() error {
		calls++
		if calls == 1 {
			return errRetryable
		}
		return errFatal
	})

	require.ErrorIs(t, err, errFatal)
	assert.Equal(t, 2, calls)
}

func TestRetryWithBackoffCapsDelay(t *testing.T) {
	errTemporary := errors.New("temporary")
	var delays []time.Duration

	err := RetryWithBackoff(context.Background(), RetryOptions{
		Attempts: 4,
		Delay:    time.Nanosecond,
		MaxDelay: 2 * time.Nanosecond,
		OnRetry: func(_ int, _ error, delay time.Duration) {
			delays = append(delays, delay)
		},
	}, func() error {
		return errTemporary
	})

	require.ErrorIs(t, err, errTemporary)
	assert.Equal(t, []time.Duration{
		time.Nanosecond,
		2 * time.Nanosecond,
		2 * time.Nanosecond,
	}, delays)
}

func TestRetryWithBackoffStopsWhenContextIsCanceled(t *testing.T) {
	errTemporary := errors.New("temporary")
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0

	err := RetryWithBackoff(ctx, RetryOptions{
		Attempts: 3,
		Delay:    time.Hour,
		OnRetry: func(_ int, _ error, _ time.Duration) {
			cancel()
		},
	}, func() error {
		calls++
		return errTemporary
	})

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, calls)
}

func TestRetryDelayWithJitter(t *testing.T) {
	base := 4 * time.Second

	for i := 0; i < 10; i++ {
		delay := retryDelay(base, 0, 0, 0.25)

		assert.GreaterOrEqual(t, delay, base)
		assert.Less(t, delay, base+time.Second)
	}
}

func TestRetryWithExpBackoffRunsAtLeastOnce(t *testing.T) {
	errTemporary := errors.New("temporary")
	calls := 0

	err := RetryWithExpBackoff(0, time.Nanosecond, func() error {
		calls++
		return errTemporary
	})

	require.ErrorIs(t, err, errTemporary)
	assert.Equal(t, 1, calls)
}
