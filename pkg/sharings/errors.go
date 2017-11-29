package sharings

import "errors"

// TODO i *instance.Instance vs db couchdb.Database on the whole pkg/sharings
// TODO does all these errors still make sense?
var (
	// ErrBadSharingType is used when the given sharing type is not valid
	ErrBadSharingType = errors.New("Invalid sharing type")
	// ErrBadPermission is used when a given permission is not valid
	ErrBadPermission = errors.New("Invalid permission format")
	//ErrRecipientDoesNotExist is used when the given recipient does not exist
	ErrRecipientDoesNotExist = errors.New("Recipient with given ID does not exist")
	//ErrMissingScope is used when a request is missing the mandatory scope
	ErrMissingScope = errors.New("The scope parameter is mandatory")
	//ErrMissingState is used when a request is missing the mandatory state
	ErrMissingState = errors.New("The state parameter is mandatory")
	//ErrMissingCode is used when a request is missing the mandatory code
	ErrMissingCode = errors.New("The code parameter is mandatory")
	// ErrSharingDoesNotExist is used when the given sharing does not exist.
	ErrSharingDoesNotExist = errors.New("Sharing does not exist")
	// ErrRecipientHasNoEmail is used to signal that a recipient has no email.
	ErrRecipientHasNoEmail = errors.New("Recipient has no email")
	// ErrRecipientHasNoURL is used to signal that a recipient has no URL.
	ErrRecipientHasNoURL = errors.New("Recipient has no URL")
	// ErrRecipientBadParams is used at a recipient creation when the params are not well defined
	ErrRecipientBadParams = errors.New("Recipient parameters are invalid")
	// ErrMailCouldNotBeSent is used when an error ocurred while trying to send
	// an email.
	ErrMailCouldNotBeSent = errors.New("Mail could not be sent")
	// ErrNoOAuthClient is used when the owner of the Cozy has not yet
	// registered to the recipient as an OAuth client.
	ErrNoOAuthClient = errors.New("No OAuth client was found")
	//ErrSharingIDNotUnique is used when several occurences of the same sharing id are found
	ErrSharingIDNotUnique = errors.New("Several sharings with this id found")
	// ErrSharingAlreadyExist is returned when the creation of a new sharing
	// is asked but the sharing_id is already used by an existing sharing.
	ErrSharingAlreadyExist = errors.New("Sharing with this sharing_id already exists")
	// ErrSharerDidNotReceiveAnswer is used when a recipient has not received a
	// http.StatusOK after sending her answer to the sharer.
	ErrSharerDidNotReceiveAnswer = errors.New("Sharer did not receive the answer")
	// ErrPublicNameNotDefined is used when a sharer wants to register to a recipient
	ErrPublicNameNotDefined = errors.New("The Cozy's public name must be defined")
	// ErrOnlySharerCanRevokeRecipient is used when a user other than the sharer
	// attempts to revoke a recipient.
	ErrOnlySharerCanRevokeRecipient = errors.New("Only the sharer can revoke " +
		"a recipient")
	// ErrForbidden is used when a request is made with insufficient rights.
	ErrForbidden = errors.New("Request denied: insufficient rights")
)
