package vfs

import (
	"errors"
	"net/http"
	"os"
)

var (
	ErrDocAlreadyExists      = os.ErrExist
	ErrDocDoesNotExist       = os.ErrNotExist
	ErrParentDoesNotExist    = errors.New("Parent folder with given FolderID does not exist")
	ErrDocTypeInvalid        = errors.New("Invalid document type")
	ErrIllegalFilename       = errors.New("Invalid filename: empty or contains an illegal character")
	ErrInvalidHash           = errors.New("Invalid hash")
	ErrContentLengthInvalid  = errors.New("Invalid content length")
	ErrContentLengthMismatch = errors.New("Content length does not match")
)

// Return the HTTP status code associated to a given error. If the
// error is not part of vfs errors, the code returned is 0.
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
