package vfs

import "errors"

var (
	// ErrParentDoesNotExist is used when the parent directory does not
	// exist
	ErrParentDoesNotExist = errors.New("parent directory with given DirID does not exist")
	// ErrForbiddenDocMove is used when trying to move a document in an
	// illicit destination
	ErrForbiddenDocMove = errors.New("forbidden document move")
	// ErrIllegalFilename is used when the given filename is not allowed
	ErrIllegalFilename = errors.New("invalid filename: empty or contains an illegal character")
	// ErrIllegalPath is used when the path has too many levels
	ErrIllegalPath = errors.New("invalid path: too many levels")
	// ErrIllegalMime is used when the mime-type of a file is invalid
	ErrIllegalMime = errors.New("invalid Content-Type")
	// ErrIllegalTime is used when a time given (creation or
	// modification) is not allowed
	ErrIllegalTime = errors.New("invalid time given")
	// ErrInvalidHash is used when the given hash does not match the
	// calculated one
	ErrInvalidHash = errors.New("invalid hash")
	// ErrContentLengthMismatch is used when the content-length does not
	// match the calculated one
	ErrContentLengthMismatch = errors.New("content length does not match")
	// ErrConflict is used when the access to a file or directory is in
	// conflict with another
	ErrConflict = errors.New("conflict access to same file or directory")
	// ErrFileInTrash is used when the file is already in the trash
	ErrFileInTrash = errors.New("file or directory is already in the trash")
	// ErrFileNotInTrash is used when the file is not in the trash
	ErrFileNotInTrash = errors.New("file or directory is not in the trash")
	// ErrParentInTrash is used when trying to upload a file to a directory
	// that is trashed
	ErrParentInTrash = errors.New("parent directory is in the trash")
	// ErrNonAbsolutePath is used when the given path is not absolute
	// while it is required to be
	ErrNonAbsolutePath = errors.New("path should be absolute")
	// ErrDirNotEmpty is used to inform that the directory is not
	// empty
	ErrDirNotEmpty = errors.New("directory is not empty")
	// ErrWrongCouchdbState is given when couchdb gives us an unexpected value
	ErrWrongCouchdbState = errors.New("wrong couchdb reduce value")
	// ErrFileTooBig is used when there is no more space left on the filesystem
	ErrFileTooBig = errors.New("the file is too big and exceeds the disk quota")
	// ErrFsckFailFast is used when the FSCK is stopped by the fail-fast option
	ErrFsckFailFast = errors.New("fSCK has been stopped on first failure")
	// ErrWrongToken is used when a key is not found on the store
	ErrWrongToken = errors.New("wrong download token")
)
