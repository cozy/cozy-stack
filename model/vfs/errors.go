package vfs

import "errors"

var (
	// ErrParentDoesNotExist is used when the parent directory does not
	// exist
	ErrParentDoesNotExist = errors.New("Parent directory with given DirID does not exist")
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
	// ErrFileNotInTrash is used when the file is not in the trash
	ErrFileNotInTrash = errors.New("File or directory is not in the trash")
	// ErrParentInTrash is used when trying to upload a file to a directory
	// that is trashed
	ErrParentInTrash = errors.New("Parent directory is in the trash")
	// ErrNonAbsolutePath is used when the given path is not absolute
	// while it is required to be
	ErrNonAbsolutePath = errors.New("Path should be absolute")
	// ErrDirNotEmpty is used to inform that the directory is not
	// empty
	ErrDirNotEmpty = errors.New("Directory is not empty")
	// ErrWrongCouchdbState is given when couchdb gives us an unexpected value
	ErrWrongCouchdbState = errors.New("Wrong couchdb reduce value")
	// ErrFileTooBig is used when there is no more space left on the filesystem
	ErrFileTooBig = errors.New("The file is too big and exceeds the disk quota")
)
