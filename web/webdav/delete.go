package webdav

import (
	"errors"
	"net/http"
	"os"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// handleDelete handles HTTP DELETE requests for soft-trashing files and
// directories. Per user decision (02-CONTEXT.md), DELETE uses vfs.TrashFile /
// vfs.TrashDir (soft-trash) — not DestroyFile / DestroyDirAndContent.
//
// Contract:
//   - File exists: trash via vfs.TrashFile, return 204 No Content
//   - Directory exists: trash entire tree via vfs.TrashDir, return 204 No Content
//   - Path not found: 404 Not Found
//   - Path inside .cozy_trash: 405 Method Not Allowed with Allow header
func handleDelete(c echo.Context) error {
	rawParam := c.Param("*")
	vfsPath, err := davPathToVFSPath(rawParam)
	if err != nil {
		auditLog(c, "delete path rejected", rawParam)
		return sendWebDAVError(c, http.StatusForbidden, "forbidden")
	}

	// Write-fence: reject DELETE inside .cozy_trash with 405.
	if isInTrash(vfsPath) {
		auditLog(c, "delete attempt in trash", vfsPath)
		c.Response().Header().Set("Allow", "PROPFIND, GET, HEAD, OPTIONS")
		return sendWebDAVError(c, http.StatusMethodNotAllowed, "method-not-allowed")
	}

	inst := middlewares.GetInstance(c)
	fs := inst.VFS()

	// Resolve the resource — could be file or directory.
	dir, file, err := fs.DirOrFileByPath(vfsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sendWebDAVError(c, http.StatusNotFound, "not-found")
		}
		return mapVFSWriteError(c, err)
	}

	// Soft-trash the resource.
	if file != nil {
		_, err = vfs.TrashFile(fs, file)
	} else {
		_, err = vfs.TrashDir(fs, dir)
	}
	if err != nil {
		return mapVFSWriteError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}
