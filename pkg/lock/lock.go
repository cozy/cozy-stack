package lock

// ReadWrite returns a read/write lock for the given name
func ReadWrite(domain string) ErrorRWLocker {
	if c := getClient(); c != nil {
		return &fakeRWLock{getReadisReadWriteLock(c, domain)}
	}
	return getMemReadWriteLock(domain)
}

// An ErrorLocker is a locker which can fail (returns an error)
type ErrorLocker interface {
	Lock() error
	Unlock() error
}

// ErrorRWLocker is the interface for a RWLock as inspired by RWMutex
type ErrorRWLocker interface {
	ErrorLocker
	RLock() error
	RUnlock() error
}
