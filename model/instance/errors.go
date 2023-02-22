package instance

import "errors"

var (
	// ErrNotFound is used when the seeked instance was not found
	ErrNotFound = errors.New("instance not found")
	// ErrExists is used the instance already exists
	ErrExists = errors.New("instance already exists")
	// ErrIllegalDomain is used when the domain named contains illegal characters
	ErrIllegalDomain = errors.New("domain name contains illegal characters")
	// ErrMissingToken is returned by RegisterPassphrase if token is empty
	ErrMissingToken = errors.New("empty register token")
	// ErrInvalidToken is returned by RegisterPassphrase if token is invalid
	ErrInvalidToken = errors.New("invalid register token")
	// ErrMissingPassphrase is returned when the new passphrase is missing
	ErrMissingPassphrase = errors.New("missing new passphrase")
	// ErrInvalidPassphrase is returned when the passphrase is invalid
	ErrInvalidPassphrase = errors.New("invalid passphrase")
	// ErrInvalidTwoFactor is returned when the two-factor authentication
	// verification is invalid.
	ErrInvalidTwoFactor = errors.New("invalid two-factor parameters")
	// ErrResetAlreadyRequested is returned when a passphrase reset token is already set and valid
	ErrResetAlreadyRequested = errors.New("the passphrase reset has already been requested")
	// ErrUnknownAuthMode is returned when an unknown authentication mode is
	// used.
	ErrUnknownAuthMode = errors.New("unknown authentication mode")
	// ErrBadTOSVersion is returned when a malformed TOS version is provided.
	ErrBadTOSVersion = errors.New("bad format for TOS version")
	// ErrInvalidSwiftLayout is returned when the Swift layout is unknown.
	ErrInvalidSwiftLayout = errors.New("invalid Swift layout")
	// ErrDeletionAlreadyRequested is returned when a deletion has already been requested.
	ErrDeletionAlreadyRequested = errors.New("the deletion has already been requested")
)
