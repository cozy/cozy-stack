package contacts

import "errors"

var (
	// ErrNoMailAddress is returned when trying to access the email address of
	// a contact that has no known email address.
	ErrNoMailAddress = errors.New("The contact has no email address")
)
