package webdav

import (
	"net/http"

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
	return sendWebDAVError(c, http.StatusNotImplemented, "not-implemented")
}
