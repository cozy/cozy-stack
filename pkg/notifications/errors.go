package notifications

import "errors"

var (
	// ErrBadNotification is used when the notification request is not valid.
	ErrBadNotification = errors.New("Notification is not valid")
)
