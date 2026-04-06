package webdav

import (
	"errors"

	"github.com/labstack/echo/v4"
)

// errETagMismatch is a sentinel returned by checkETagPreconditions when the
// If-Match or If-None-Match header does not match the file's current ETag.
var errETagMismatch = errors.New("webdav: etag precondition failed")

// isInTrash reports whether vfsPath is inside the .cozy_trash directory tree.
// Used as a write-fence: PUT/DELETE/MKCOL/MOVE into trash are forbidden via
// WebDAV (trash is system-managed, see 02-CONTEXT.md).
func isInTrash(_ string) bool {
	// Stub: always returns false (RED state — tests must fail).
	return false
}

// mapVFSWriteError maps a VFS error to the appropriate HTTP status and
// RFC 4918 error XML body via sendWebDAVError. Callers use it after any
// VFS write operation (CreateFile, TrashFile, TrashDir, Mkdir, ModifyFileMetadata).
//
// The error is returned unmodified if it does not match any known VFS sentinel,
// letting Echo's default error handler surface a 500.
func mapVFSWriteError(_ echo.Context, err error) error {
	// Stub: pass-through (RED state — tests must fail).
	return err
}

// checkETagPreconditions validates If-Match and If-None-Match headers against
// an existing file's ETag. Returns errETagMismatch if the precondition fails,
// nil otherwise.
func checkETagPreconditions(_ echo.Context, _ interface{}) error {
	// Stub: always returns nil (RED state — tests must fail).
	return nil
}
