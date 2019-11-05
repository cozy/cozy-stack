package note

import "errors"

var (
	// ErrInvalidSchema is used when the schema cannot be read by prosemirror.
	ErrInvalidSchema = errors.New("Invalid schema for prosemirror")
)
