package nextcloud

import "errors"

var (
	// ErrAccountNotFound is used when the no account can be found with the
	// given ID.
	ErrAccountNotFound = errors.New("account not found")
	// ErrInvalidAccount is used when the account cannot be used to connect to
	// NextCloud.
	ErrInvalidAccount = errors.New("invalid NextCloud account")
)
