package note

import "errors"

var (
	// ErrInvalidSchema is used when the schema cannot be read by prosemirror.
	ErrInvalidSchema = errors.New("Invalid schema for prosemirror")
	// ErrInvalidFile is used when a file doesn't have the metadata to be used
	// as a note.
	ErrInvalidFile = errors.New("Invalid file, not a note")
	// ErrNoSteps is used when steps are expected, but there are none.
	ErrNoSteps = errors.New("No steps")
	// ErrInvalidSteps is used when prosemirror can't instantiate the steps.
	ErrInvalidSteps = errors.New("Invalid steps")
	// ErrCannotApply is used when trying to apply some steps, but it fails
	// because of a conflict. The client can try to rebase the steps.
	ErrCannotApply = errors.New("Cannot apply the steps")
	// ErrTooOld is used when the steps just after the given revision are no
	// longer available.
	ErrTooOld = errors.New("The revision is too old")
	// ErrMissingSessionID is used when a telepointer has no identifier.
	ErrMissingSessionID = errors.New("The session id is missing")
)
