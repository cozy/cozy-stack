package apps

import "errors"

var (
	// ErrInvalidSlugName is used when the given slug name is not valid
	ErrInvalidSlugName = errors.New("Invalid slug name")
	// ErrAlreadyExists is used when an application with the specified slug name
	// is already installed.
	ErrAlreadyExists = errors.New("Application with same slug already exists")
	// ErrNotFound is used when no application with specified slug name is
	// installed.
	ErrNotFound = errors.New("Application is not installed")
	// ErrNotSupportedSource is used when the source transport or
	// protocol is not supported
	ErrNotSupportedSource = errors.New("Invalid or not supported source scheme")
	// ErrManifestNotReachable is used when the manifest of the
	// application is not reachable
	ErrManifestNotReachable = errors.New("Application manifest " + ManifestFilename + " is not reachable")
	// ErrSourceNotReachable is used when the given source for
	// application is not reachable
	ErrSourceNotReachable = errors.New("Application source is not reachable")
	// ErrBadManifest when the manifest is not valid or malformed
	ErrBadManifest = errors.New("Application manifest is invalid or malformed")
	// ErrBadState is used when trying to use the application while in a
	// state that is not appropriate for the given operation.
	ErrBadState = errors.New("Application is not in valid state to perform this operation")
)
