package lock

import "github.com/cozy/cozy-stack/pkg/config"

// ReadWrite returns the read/write lock for the given name
func ReadWrite(domain string) ErrorRWLocker {
	cli := config.GetConfig().Lock.Client()
	if cli != nil {
		return getRedisReadWriteLock(cli, domain)
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
