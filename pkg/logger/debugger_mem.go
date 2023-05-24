package logger

import (
	"sync"
	"time"
)

// MemDebugger is a in-memory based [Debugger] implementation.
//
// This implem is local only and is not suited for any multi-instance setup.
type MemDebugger struct {
	domains *sync.Map
}

// NewMemDebugger instantiate a new [MemDebugger].
func NewMemDebugger() *MemDebugger {
	return &MemDebugger{domains: new(sync.Map)}
}

// AddDomain adds the specified domain to the debug list.
func (m *MemDebugger) AddDomain(domain string, ttl time.Duration) error {
	m.domains.Store(domain, time.Now().Add(ttl))
	return nil
}

// RemoveDomain removes the specified domain from the debug list.
func (m *MemDebugger) RemoveDomain(domain string) error {
	m.domains.Delete(domain)
	return nil
}

// ExpiresAt returns the expiration time for this domain.
//
// If this domain is not in debug mode, it returns `nil`.
func (m *MemDebugger) ExpiresAt(domain string) *time.Time {
	res, ok := m.domains.Load(domain)
	if !ok {
		return nil
	}

	t := res.(time.Time)

	if time.Now().After(t) {
		m.RemoveDomain(domain)
		return nil
	}

	return &t
}
