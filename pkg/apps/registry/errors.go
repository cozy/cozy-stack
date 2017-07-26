package registry

import "errors"

var (
	// ErrAppNotFound is used when an app is asked but does not exist in
	// the registries
	ErrAppNotFound = errors.New("Application was not found")
	// ErrVersionNotFound is used when a version is asked but does not exist in
	// the registries
	ErrVersionNotFound = errors.New("Version was not found")
)
