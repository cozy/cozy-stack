package lock

import (
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// ReadWrite returns the read/write lock for the given name.
// By convention, the name should be prefixed by the instance domain on which
// it applies, then a slash and the package name (ie alice.example.net/vfs).
func ReadWrite(db prefixer.Prefixer, name string) ErrorRWLocker {
	cli := config.GetConfig().Lock.Client()
	if cli != nil {
		return getRedisReadWriteLock(cli, db.DBPrefix()+"/"+name)
	}
	return getMemReadWriteLock(name)
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
