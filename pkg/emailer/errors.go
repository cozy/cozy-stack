package emailer

import "errors"

var (
	// ErrMissingContent is used when sending an email without parts
	ErrMissingContent = errors.New("emailer: missing content")
	// ErrMissingSubject is used when sending an email without subject
	ErrMissingSubject = errors.New("emailer: missing subject")
	// ErrMissingTemplate is used when sending an email without a template name
	// or template values
	ErrMissingTemplate = errors.New("emailer: missing template")
)
