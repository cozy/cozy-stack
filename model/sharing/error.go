package sharing

import "errors"

var (
	// ErrNoRules is used when a sharing is created without a rule
	ErrNoRules = errors.New("a sharing must have rules")
	// ErrNoRecipients is used when a sharing is created without a recipient
	ErrNoRecipients = errors.New("a sharing must have recipients")
	// ErrTooManyMembers is used when a sharing has too many members
	ErrTooManyMembers = errors.New("there are too many members for this sharing")
	// ErrInvalidURL is used for invalid URL of a Cozy instance
	ErrInvalidURL = errors.New("the Cozy URL is invalid")
	// ErrInvalidRule is used when a rule is invalid when the sharing is
	// created
	ErrInvalidRule = errors.New("a rule is invalid")
	// ErrInvalidSharing is used when an action cannot be made on a sharing,
	// because this sharing is not the expected state
	ErrInvalidSharing = errors.New("sharing is not in the expected state")
	// ErrMemberNotFound is used when trying to find a member, but there is no
	// member with the expected value for the criterion
	ErrMemberNotFound = errors.New("the member was not found")
	// ErrInvitationNotSent is used when the invitation shortcut or mail failed
	// to be sent
	ErrInvitationNotSent = errors.New("the invitation cannot be sent")
	// ErrRequestFailed is used when a cozy tries to create a sharing request
	// on another cozy, but it failed
	ErrRequestFailed = errors.New("the sharing request failed")
	// ErrNoOAuthClient is used when the owner of the Cozy has not yet
	// registered to the recipient as an OAuth client.
	ErrNoOAuthClient = errors.New("no OAuth client was found")
	// ErrInternalServerError is used for CouchDB errors
	ErrInternalServerError = errors.New("internal Server Error")
	// ErrClientError is used when an OAuth client has made a request, and the
	// response was a 4xx error
	ErrClientError = errors.New("oAuth client request was in error")
	// ErrMissingID is used when _id is missing on a doc for a bulk operation
	ErrMissingID = errors.New("an identifier is missing")
	// ErrMissingRev is used when _rev is missing on a doc for a bulk operation
	ErrMissingRev = errors.New("a revision is missing")
	// ErrMissingFileMetadata is used when uploading a file and the key is not
	// in the cache (so no metadata and the upload can't succeed)
	ErrMissingFileMetadata = errors.New("the metadata for this file were not found")
	// ErrFolderNotFound is used when informations about a folder is asked,
	// but this folder was not found
	ErrFolderNotFound = errors.New("this folder was not found")
	// ErrSafety is used when an operation is aborted due to the safety principal
	ErrSafety = errors.New("operation aborted")
	// ErrAlreadyAccepted is used when someone tries to accept twice a sharing
	// on the same cozy instance
	ErrAlreadyAccepted = errors.New("sharing already accepted by this recipient")
	// ErrCannotOpenFile is used when opening a file fails
	ErrCannotOpenFile = errors.New("the file cannot be opened")
)
