package lock

import "sync"

var locks map[string]*memLock
var locksMu sync.Mutex

type memLock struct {
	sync.RWMutex
}

func (ml *memLock) Lock() error    { ml.RWMutex.Lock(); return nil }
func (ml *memLock) RLock() error   { ml.RWMutex.RLock(); return nil }
func (ml *memLock) Unlock() error  { ml.RWMutex.Unlock(); return nil }
func (ml *memLock) RUnlock() error { ml.RWMutex.RUnlock(); return nil }

// getMemReadWriteLock returns a sync.RWMutex.
func getMemReadWriteLock(domain string) ErrorRWLocker {
	locksMu.Lock()
	defer locksMu.Unlock()
	if locks == nil {
		locks = make(map[string]*memLock)
	}
	l, ok := locks[domain]
	if !ok {
		l = &memLock{}
		locks[domain] = l
	}
	return l
}
