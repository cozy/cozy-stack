package webdav

import (
	"errors"
	"net/http"
	"os"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// handleMkcol handles MKCOL requests for single-directory creation.
//
// Contract (RFC 4918 section 9.3):
//   - New directory created: 201 Created
//   - Path already exists: 405 Method Not Allowed
//   - Parent directory missing: 409 Conflict
//   - Request body present: 415 Unsupported Media Type (extended MKCOL not supported)
//   - Path inside .cozy_trash: 403 Forbidden (trash is system-managed)
//
// Uses vfs.Mkdir (single-level only, NOT MkdirAll) to avoid a known
// race condition in cozy-stack (see CONCERNS.md).
func handleMkcol(c echo.Context) error {
	// RFC 4918 section 9.3: MKCOL with a request body MUST return 415 if
	// the server does not understand the body type. Since we do not support
	// extended MKCOL, any body is rejected.
	if c.Request().ContentLength > 0 {
		return sendWebDAVError(c, http.StatusUnsupportedMediaType, "unsupported-media-type")
	}
	// Also handle chunked encoding where ContentLength is -1 but body exists.
	if c.Request().ContentLength < 0 && c.Request().Body != nil {
		buf := make([]byte, 1)
		if n, _ := c.Request().Body.Read(buf); n > 0 {
			return sendWebDAVError(c, http.StatusUnsupportedMediaType, "unsupported-media-type")
		}
	}

	rawParam := c.Param("*")
	vfsPath, err := davPathToVFSPath(rawParam)
	if err != nil {
		auditLog(c, "mkcol path rejected", rawParam)
		return sendWebDAVError(c, http.StatusForbidden, "forbidden")
	}

	// Write-fence: reject directory creation inside .cozy_trash.
	if isInTrash(vfsPath) {
		auditLog(c, "mkcol into trash rejected", vfsPath)
		return sendWebDAVError(c, http.StatusForbidden, "forbidden")
	}

	inst := middlewares.GetInstance(c)
	_, err = vfs.Mkdir(inst.VFS(), vfsPath, nil)
	if err != nil {
		// vfs.Mkdir calls fs.DirByPath(parentDir) which returns os.ErrNotExist
		// when the parent is missing — not vfs.ErrParentDoesNotExist. Per
		// RFC 4918 section 9.3, a missing parent MUST be 409 Conflict.
		if errors.Is(err, os.ErrNotExist) {
			return sendWebDAVError(c, http.StatusConflict, "conflict")
		}
		return mapVFSWriteError(c, err)
	}

	return c.NoContent(http.StatusCreated)
}
