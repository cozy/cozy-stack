package vfs

import "sync"

var locks map[string]*sync.RWMutex
var locksMu sync.Mutex

// NewMemLock returns a sync.RWMutex.
func NewMemLock(domain string) Locker {
	locksMu.Lock()
	defer locksMu.Unlock()
	if locks == nil {
		locks = make(map[string]*sync.RWMutex)
	}
	l, ok := locks[domain]
	if !ok {
		l = &sync.RWMutex{}
		locks[domain] = l
	}
	return l
}
