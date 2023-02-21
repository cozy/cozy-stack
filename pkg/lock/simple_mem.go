package lock

import (
	"sync"

	"github.com/cozy/cozy-stack/pkg/prefixer"
)

var locks map[string]*memLock
var locksMu sync.Mutex

type InMemoryLockGetter struct {
	locks *sync.Map
}

func NewInMemory() *InMemoryLockGetter {
	return &InMemoryLockGetter{locks: new(sync.Map)}
}

func (i *InMemoryLockGetter) ReadWrite(_ prefixer.Prefixer, name string) ErrorRWLocker {
	lock, _ := i.locks.LoadOrStore(name, &memLock{})

	return lock.(*memLock)
}

// LongOperation returns a lock suitable for long operations. It will refresh
// the lock in redis to avoid its automatic expiration.
func (i *InMemoryLockGetter) LongOperation(db prefixer.Prefixer, name string) ErrorLocker {
	return &longOperation{
		lock: i.ReadWrite(db, name),
	}
}

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
