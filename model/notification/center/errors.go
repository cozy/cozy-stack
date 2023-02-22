package center

import "errors"

var (
	// ErrBadNotification is used when the notification request is not valid.
	ErrBadNotification = errors.New("notification is not valid")
	// ErrUnauthorized is used when the notification creator is not authorized to do so.
	ErrUnauthorized = errors.New("not authorized to send notification")
	// ErrNoCategory is used when no category is declared for this application
	ErrNoCategory = errors.New("no category for this app")
	// ErrCategoryNotFound is used when sending a notification from an unknown
	// category.
	ErrCategoryNotFound = errors.New("notification category does not exist")
)
