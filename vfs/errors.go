package vfs

import (
	"errors"
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
