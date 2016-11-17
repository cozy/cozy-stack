package vfs

import "errors"

var (
	// ErrParentDoesNotExist is used when the parent folder does not
	// exist
	ErrParentDoesNotExist = errors.New("Parent folder with given FolderID does not exist")
	// ErrForbiddenDocMove is used when trying to move a document in an
	// illicit destination
	ErrForbiddenDocMove = errors.New("Forbidden document move")
	// ErrIllegalFilename is used when the given filename is not allowed
	ErrIllegalFilename = errors.New("Invalid filename: empty or contains an illegal character")
	// ErrIllegalTime is used when a time given (creation or
	// modification) is not allowed
	ErrIllegalTime = errors.New("Invalid time given")
	// ErrInvalidHash is used when the given hash does not match the
	// calculated one
	ErrInvalidHash = errors.New("Invalid hash")
	// ErrContentLengthMismatch is used when the content-length does not
	// match the calculated one
	ErrContentLengthMismatch = errors.New("Content length does not match")
	// ErrConflict is used when the access to a file or directory is in
	// conflict with another
	ErrConflict = errors.New("Conflict access to same file or directory")
	// ErrFileInTrash is used when the file is already in the trash
	ErrFileInTrash = errors.New("File or directory is already in the trash")
)
