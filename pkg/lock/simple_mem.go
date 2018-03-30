package lock

import "sync"

var locks map[string]*memLock
var locksMu sync.Mutex

type memLock struct {
	sync.RWMutex
}

func (ml *memLock) Lock() error  { ml.RWMutex.Lock(); return nil }
func (ml *memLock) RLock() error { ml.RWMutex.RLock(); return nil }
func (ml *memLock) Unlock()      { ml.RWMutex.Unlock() }
func (ml *memLock) RUnlock()     { ml.RWMutex.RUnlock() }

// getMemReadWriteLock returns a sync.RWMutex.
func getMemReadWriteLock(name string) ErrorRWLocker {
	locksMu.Lock()
	defer locksMu.Unlock()
	if locks == nil {
		locks = make(map[string]*memLock)
	}
	l, ok := locks[name]
	if !ok {
		l = &memLock{}
		locks[name] = l
	}
	return l
}
