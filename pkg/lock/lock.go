package lock

// ReadWrite returns the read/write lock for the given name
func ReadWrite(domain string) ErrorRWLocker {
	if c := getClient(); c != nil {
		return &fakeRWLock{getRedisReadWriteLock(c, domain)}
	}
	return getMemReadWriteLock(domain)
}

// An ErrorLocker is a locker which can fail (returns an error)
type ErrorLocker interface {
	Lock() error
	Unlock()
}

// ErrorRWLocker is the interface for a RWLock as inspired by RWMutex
type ErrorRWLocker interface {
	ErrorLocker
	RLock() error
	RUnlock()
}
