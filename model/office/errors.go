package office

import "errors"

var (
	// ErrInvalidFile is used when a file is not an office document
	ErrInvalidFile = errors.New("Invalid file, not an office document")
	// ErrInternalServerError is used when something goes wrong (like no
	// connection to redis)
	ErrInternalServerError = errors.New("Internal server error")
)
