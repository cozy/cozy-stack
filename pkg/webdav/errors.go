package webdav

import "errors"

var (
	// ErrInvalidAuth is used when an authentication error occurs (invalid
	// credentials).
	ErrInvalidAuth = errors.New("invalid authentication")
	// ErrAlreadyExist is used when trying to create a directory that already
	// exists.
	ErrAlreadyExist = errors.New("it already exists")
	// ErrParentNotFound is used when trying to create a directory and the
	// parent directory does not exist.
	ErrParentNotFound = errors.New("parent directory does not exist")
	// ErrNotFound is used when the given file/directory has not been found.
	ErrNotFound = errors.New("file/directory not found")
	// ErrInternalServerError is used when something unexpected happens.
	ErrInternalServerError = errors.New("internal server error")
)
