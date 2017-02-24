package sharings

import "errors"

var (
	// ErrBadSharingType is used when the given sharing type is not valid
	ErrBadSharingType = errors.New("Invalid sharing type")
	//ErrRecipientDoesNotExist is used when the given recipient does not exist
	ErrRecipientDoesNotExist = errors.New("Recipient with given ID does not exist")
	//ErrMissingScope is used when a request is missing the mandatory scope
	ErrMissingScope = errors.New("The scope parameter is mandatory")
	//ErrMissingState is used when a request is missing the mandatory state
	ErrMissingState = errors.New("The state parameter is mandatory")
)
