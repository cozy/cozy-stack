package webdav

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/labstack/echo/v4"
)

// errETagMismatch is a sentinel returned by checkETagPreconditions when the
// If-Match or If-None-Match header does not match the file's current ETag.
var errETagMismatch = errors.New("webdav: etag precondition failed")

// isInTrash reports whether vfsPath is inside the .cozy_trash directory tree.
// Used as a write-fence: PUT/DELETE/MKCOL/MOVE into trash are forbidden via
// WebDAV (trash is system-managed, see 02-CONTEXT.md).
func isInTrash(vfsPath string) bool {
	return vfsPath == vfs.TrashDirName || strings.HasPrefix(vfsPath, vfs.TrashDirName+"/")
}

// mapVFSWriteError maps a VFS error to the appropriate HTTP status and
// RFC 4918 error XML body via sendWebDAVError. Callers use it after any
// VFS write operation (CreateFile, TrashFile, TrashDir, Mkdir, ModifyFileMetadata).
//
// The error is returned unmodified if it does not match any known VFS sentinel,
// letting Echo's default error handler surface a 500.
func mapVFSWriteError(c echo.Context, err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, vfs.ErrFileTooBig), errors.Is(err, vfs.ErrMaxFileSize):
		auditLog(c, "quota exceeded", c.Request().URL.Path)
		return sendWebDAVError(c, http.StatusInsufficientStorage, "quota-not-exceeded")

	case errors.Is(err, vfs.ErrParentDoesNotExist), errors.Is(err, vfs.ErrParentInTrash):
		return sendWebDAVError(c, http.StatusConflict, "conflict")

	case errors.Is(err, vfs.ErrForbiddenDocMove):
		auditLog(c, "forbidden move", c.Request().URL.Path)
		return sendWebDAVError(c, http.StatusForbidden, "forbidden")

	case errors.Is(err, vfs.ErrFileInTrash):
		return sendWebDAVError(c, http.StatusMethodNotAllowed, "method-not-allowed")

	case errors.Is(err, os.ErrNotExist):
		return sendWebDAVError(c, http.StatusNotFound, "not-found")

	case errors.Is(err, os.ErrExist):
		return sendWebDAVError(c, http.StatusMethodNotAllowed, "method-not-allowed")

	default:
		return err
	}
}

// checkETagPreconditions validates If-Match and If-None-Match headers against
// an existing file's ETag. Returns errETagMismatch if the precondition fails,
// nil otherwise. existingFile must be non-nil.
func checkETagPreconditions(c echo.Context, existingFile *vfs.FileDoc) error {
	currentETag := buildETag(existingFile.MD5Sum)

	if ifMatch := c.Request().Header.Get("If-Match"); ifMatch != "" {
		if ifMatch != currentETag {
			return errETagMismatch
		}
	}

	if ifNoneMatch := c.Request().Header.Get("If-None-Match"); ifNoneMatch == "*" {
		// If-None-Match: * means "only if the resource does NOT exist".
		// Since existingFile is non-nil, the precondition fails.
		return errETagMismatch
	}

	return nil
}
