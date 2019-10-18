package utils

import "time"

// RetryWithExpBackoff can be used to call several times a function until it
// returns no error or the maximum count of calls has been reached. Between two
// calls, it will wait, first by the given delay, and after that, the delay
// will double after each failure.
func RetryWithExpBackoff(count int, delay time.Duration, fn func() error) error {
	err := fn()
	if err == nil {
		return nil
	}
	for i := 1; i < count; i++ {
		time.Sleep(delay)
		delay *= 2
		err = fn()
		if err == nil {
			return nil
		}
	}
	return err
}
