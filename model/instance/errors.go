package instance

import "errors"

var (
	// ErrNotFound is used when the seeked instance was not found
	ErrNotFound = errors.New("Instance not found")
	// ErrExists is used the instance already exists
	ErrExists = errors.New("Instance already exists")
	// ErrIllegalDomain is used when the domain named contains illegal characters
	ErrIllegalDomain = errors.New("Domain name contains illegal characters")
	// ErrMissingToken is returned by RegisterPassphrase if token is empty
	ErrMissingToken = errors.New("Empty register token")
	// ErrInvalidToken is returned by RegisterPassphrase if token is invalid
	ErrInvalidToken = errors.New("Invalid register token")
	// ErrMissingPassphrase is returned when the new passphrase is missing
	ErrMissingPassphrase = errors.New("Missing new passphrase")
	// ErrInvalidPassphrase is returned when the passphrase is invalid
	ErrInvalidPassphrase = errors.New("Invalid passphrase")
	// ErrInvalidTwoFactor is returned when the two-factor authentication
	// verification is invalid.
	ErrInvalidTwoFactor = errors.New("Invalid two-factor parameters")
	// ErrContextNotFound is returned when the instance has no context
	ErrContextNotFound = errors.New("Context not found")
	// ErrResetAlreadyRequested is returned when a passphrase reset token is already set and valid
	ErrResetAlreadyRequested = errors.New("The passphrase reset has already been requested")
	// ErrUnknownAuthMode is returned when an unknown authentication mode is
	// used.
	ErrUnknownAuthMode = errors.New("Unknown authentication mode")
	// ErrBadTOSVersion is returned when a malformed TOS version is provided.
	ErrBadTOSVersion = errors.New("Bad format for TOS version")
)
