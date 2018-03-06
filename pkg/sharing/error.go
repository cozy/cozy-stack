package sharing

import "errors"

var (
	// ErrNoRules is used when a sharing is created without a rule
	ErrNoRules = errors.New("A sharing must have rules")
	// ErrNoRecipients is used when a sharing is created without a recipient
	ErrNoRecipients = errors.New("A sharing must have recipients")
	// ErrInvalidURL is used for invalid URL of a Cozy instance
	ErrInvalidURL = errors.New("The Cozy URL is invalid")
	// ErrInvalidSharing is used when an action cannot be made on a sharing,
	// because this sharing is not the expected state
	ErrInvalidSharing = errors.New("Sharing is not in the expected state")
	// ErrMemberNotFound is used when trying to find a member, but there is no
	// member with the expected value for the criterion
	ErrMemberNotFound = errors.New("The member was not found")
	// ErrMailNotSent is used when the invitation mail failed to be sent
	ErrMailNotSent = errors.New("The mail cannot be sent")
)
