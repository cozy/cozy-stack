package sharing

import "errors"

var (
	// ErrNoRules is used when a sharing is created without a rule
	ErrNoRules = errors.New("A sharing must have rules")
	// ErrNoRecipients is used when a sharing is created without a recipient
	ErrNoRecipients = errors.New("A sharing must have recipients")
)
