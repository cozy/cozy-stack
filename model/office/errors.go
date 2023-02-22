package office

import "errors"

var (
	// ErrNoServer is used when no OnlyOnffice server is configured for the
	// current context
	ErrNoServer = errors.New("no OnlyOnffice server is configured")
	// ErrInvalidFile is used when a file is not an office document
	ErrInvalidFile = errors.New("invalid file, not an office document")
	// ErrInternalServerError is used when something goes wrong (like no
	// connection to redis)
	ErrInternalServerError = errors.New("internal server error")
)
