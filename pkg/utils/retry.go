package utils

import (
	"context"
	"math/rand/v2"
	"time"
)

const maxRetryDelay = time.Duration(1<<63 - 1)

// RetryOptions configures RetryWithBackoff.
type RetryOptions struct {
	// Attempts is the maximum number of calls to fn. Values lower than 1 are
	// treated as 1.
	Attempts int
	// Delay is the wait duration before the first retry. It doubles after each
	// failed attempt.
	Delay time.Duration
	// MaxDelay caps the exponential backoff delay before jitter is applied.
	MaxDelay time.Duration
	// JitterFactor adds a random delay between 0 and the current backoff delay
	// multiplied by JitterFactor. This jitter is one-sided: it can only extend
	// the delay, never shorten it. For example, 0.25 adds up to 25% jitter.
	// Values lower than or equal to 0 disable jitter.
	JitterFactor float64
	// ShouldRetry can be used to retry only some errors. When nil, every
	// non-nil error is retried until Attempts is exhausted.
	ShouldRetry func(error) bool
	// OnRetry is called after a failed attempt and before sleeping. The attempt
	// argument is the number of attempts already made, starting at 1.
	OnRetry func(attempt int, err error, delay time.Duration)
}

// RetryWithBackoff calls fn until it succeeds, ShouldRetry rejects the returned
// error, the context is done, or the maximum number of attempts is reached.
func RetryWithBackoff(ctx context.Context, opts RetryOptions, fn func() error) error {
	_, err := RetryWithBackoffValue(ctx, opts, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}

// RetryWithBackoffValue calls fn until it succeeds, ShouldRetry rejects the
// returned error, the context is done, or the maximum number of attempts is
// reached. It returns the value returned by the successful call.
func RetryWithBackoffValue[T any](ctx context.Context, opts RetryOptions, fn func() (T, error)) (T, error) {
	var zero T
	if ctx == nil {
		ctx = context.Background()
	}

	attempts := opts.Attempts
	if attempts < 1 {
		attempts = 1
	}

	var err error
	var value T
	for attempt := 0; attempt < attempts; attempt++ {
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		default:
		}

		value, err = fn()
		if err == nil {
			return value, nil
		}
		if attempt == attempts-1 {
			return zero, err
		}
		if opts.ShouldRetry != nil && !opts.ShouldRetry(err) {
			return zero, err
		}

		delay := retryDelay(opts.Delay, attempt, opts.MaxDelay, opts.JitterFactor)
		if opts.OnRetry != nil {
			opts.OnRetry(attempt+1, err, delay)
		}
		if err := sleepWithContext(ctx, delay); err != nil {
			return zero, err
		}
	}

	return zero, err
}

// RetryWithExpBackoff can be used to call several times a function until it
// returns no error or the maximum count of calls has been reached. Between two
// calls, it will wait, first by the given delay, and after that, the delay
// will double after each failure.
func RetryWithExpBackoff(count int, delay time.Duration, fn func() error) error {
	return RetryWithBackoff(context.Background(), RetryOptions{
		Attempts: count,
		Delay:    delay,
	}, fn)
}

func retryDelay(initial time.Duration, attempt int, maxDelay time.Duration, jitterFactor float64) time.Duration {
	delay := backoffDelay(initial, attempt, maxDelay)
	return addJitter(delay, jitterFactor)
}

func backoffDelay(initial time.Duration, attempt int, maxDelay time.Duration) time.Duration {
	if initial <= 0 {
		return 0
	}

	delay := initial
	for i := 0; i < attempt; i++ {
		if delay > maxRetryDelay/2 {
			return cappedDelay(maxRetryDelay, maxDelay)
		}
		delay *= 2
	}
	return cappedDelay(delay, maxDelay)
}

func cappedDelay(delay time.Duration, maxDelay time.Duration) time.Duration {
	if maxDelay > 0 && delay > maxDelay {
		return maxDelay
	}
	return delay
}

func addJitter(delay time.Duration, jitterFactor float64) time.Duration {
	if delay <= 0 || jitterFactor <= 0 {
		return delay
	}

	maxJitterFloat := float64(delay) * jitterFactor
	if maxJitterFloat < 1 {
		return delay
	}
	var maxJitter time.Duration
	if maxJitterFloat >= float64(maxRetryDelay) {
		maxJitter = maxRetryDelay
	} else {
		maxJitter = time.Duration(maxJitterFloat)
	}
	if maxJitter <= 0 {
		return delay
	}
	jitter := time.Duration(rand.Int64N(int64(maxJitter)))
	if maxRetryDelay-delay < jitter {
		return maxRetryDelay
	}
	return delay + jitter
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
