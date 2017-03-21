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
	// ErrSharingDoesNotExist is used when the given sharing does not exist.
	ErrSharingDoesNotExist = errors.New("Sharing does not exist")
	// ErrRecipientHasNoEmail is used to signal that a recipient has no email.
	ErrRecipientHasNoEmail = errors.New("Recipient has no email")
	// ErrRecipientHasNoURL is used to signal that a recipient has no URL.
	ErrRecipientHasNoURL = errors.New("Recipient has no URL")
	// ErrMailCouldNotBeSent is used when an error ocurred while trying to send
	// an email.
	ErrMailCouldNotBeSent = errors.New("Mail could not be sent")
	// ErrNoOAuthClient is used when the owner of the Cozy has not yet
	// registered to the recipient as an OAuth client.
	ErrNoOAuthClient = errors.New("No OAuth client was found")
	//ErrSharingIDNotUnique is used when several occurences of the same sharing id are found
	ErrSharingIDNotUnique = errors.New("Several sharings with this id found")
	// ErrSharerDidNotReceiveAnswer is used when a recipient has not received a
	// http.StatusOK after sending her answer to the sharer.
	ErrSharerDidNotReceiveAnswer = errors.New("Sharer did not receive the answer")
	//ErrPublicNameNotDefined is used when a sharer wants to register to a recipient
	ErrPublicNameNotDefined = errors.New("The Cozy's public name must be defined")
)
