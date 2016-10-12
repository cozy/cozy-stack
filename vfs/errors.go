package vfs

import (
	"errors"
	"net/http"
	"os"
)

var (
	// ErrDocAlreadyExists is used when file or directory already exists
	ErrDocAlreadyExists = os.ErrExist
	// ErrDocDoesNotExist is used when file or directory does not exist
	ErrDocDoesNotExist = os.ErrNotExist
	// ErrParentDoesNotExist is used when the parent folder does not
	// exist
	ErrParentDoesNotExist = errors.New("Parent folder with given FolderID does not exist")
	// ErrDocTypeInvalid is used when the document type sent is not
	// recognized
	ErrDocTypeInvalid = errors.New("Invalid document type")
	// ErrIllegalFilename is used when the given filename is not allowed
	ErrIllegalFilename = errors.New("Invalid filename: empty or contains an illegal character")
	// ErrInvalidHash is used when the given hash does not match the
	// calculated one
	ErrInvalidHash = errors.New("Invalid hash")
	// ErrContentLengthInvalid is used when the content-length is not
	// valid
	ErrContentLengthInvalid = errors.New("Invalid content length")
	// ErrContentLengthMismatch is used when the content-length does not
	// match the calculated one
	ErrContentLengthMismatch = errors.New("Content length does not match")
)

// HTTPStatus returns the HTTP status code associated to a given
// error. If the error is not part of vfs errors, the code returned is
// 0.
func HTTPStatus(err error) (code int) {
	switch err {
	case ErrDocAlreadyExists:
		code = http.StatusConflict
	case ErrDocDoesNotExist:
		code = http.StatusNotFound
	case ErrParentDoesNotExist:
		code = http.StatusNotFound
	case ErrDocTypeInvalid:
	case ErrIllegalFilename:
	case ErrInvalidHash:
		code = http.StatusPreconditionFailed
	case ErrContentLengthInvalid:
		code = http.StatusUnprocessableEntity
	case ErrContentLengthMismatch:
		code = http.StatusPreconditionFailed
	}
	return
}
