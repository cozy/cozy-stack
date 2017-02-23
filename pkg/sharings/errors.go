package sharings

import "errors"

var (
	// ErrBadSharingType is used when the given sharing type is not valid
	ErrBadSharingType = errors.New("Invalid sharing type")
	//ErrRecipientDoesNotExist is used when the given recipient does not exist
	ErrRecipientDoesNotExist = errors.New("Recipient with given ID does not exist")
)
