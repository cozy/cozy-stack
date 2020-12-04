package center

import "errors"

var (
	// ErrBadNotification is used when the notification request is not valid.
	ErrBadNotification = errors.New("Notification is not valid")
	// ErrUnauthorized is used when the notification creator is not authorized to do so.
	ErrUnauthorized = errors.New("Not authorized to send notification")
	// ErrNoCategory is used when no category is declared for this application
	ErrNoCategory = errors.New("No category for this app")
	// ErrCategoryNotFound is used when sending a notification from an unknown
	// category.
	ErrCategoryNotFound = errors.New("Notification category does not exist")
)
