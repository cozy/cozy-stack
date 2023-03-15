package logger

import (
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var debugger Debugger

// Debugger manage the list of domains with the debug mode.
//
// Once you call `AddDomain` all Debug logs containing the
// corresponding `domain` field (setup with `WithDomain`)
// will be printed even if the global logger is setup with
// a higher level (like 'info').
type Debugger interface {
	AddDomain(domain string, ttl time.Duration) error
	RemoveDomain(domain string) error
	ExpiresAt(domain string) *time.Time
}

func initDebugger(client redis.UniversalClient) error {
	var err error

	if client == nil {
		debugger = NewMemDebugger()
		return nil
	}

	debugger, err = NewRedisDebugger(client)
	if err != nil {
		return fmt.Errorf("failed to init the redis debugger: %w", err)
	}

	return nil
}
