package notificationscenter

import "errors"

var (
	// ErrBadNotification is used when the notification request is not valid.
	ErrBadNotification = errors.New("Notification is not valid")
	// ErrUnauthorized is used when the notification creator is not authorized to do so.
	ErrUnauthorized = errors.New("Not authorized to send notification")
)
