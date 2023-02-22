package contact

import "errors"

var (
	// ErrNoMailAddress is returned when trying to access the email address of
	// a contact that has no known email address.
	ErrNoMailAddress = errors.New("the contact has no email address")
	// ErrNotFound is returned when no contact has been found for a query
	ErrNotFound = errors.New("no contact has been found")
)
