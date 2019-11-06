package note

import "errors"

var (
	// ErrInvalidSchema is used when the schema cannot be read by prosemirror.
	ErrInvalidSchema = errors.New("Invalid schema for prosemirror")
	// ErrInvalidFile is used when a file doesn't have the metadata to be used
	// as a note.
	ErrInvalidFile = errors.New("Invalid file, not a note")
)
