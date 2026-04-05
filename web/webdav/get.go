package webdav

import (
	"errors"
	"net/http"
	"os"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// handleGet serves a file via vfs.ServeFileContent, which wraps
// http.ServeContent and therefore handles HEAD, Range, ETag, and
// Content-Length correctly for free. GET on a collection returns 405
// Method Not Allowed per the Phase 1 read-only scope decision
// (CONTEXT.md: no HTML navigation page, READ-10).
//
// Contract:
//   - File: delegate to vfs.ServeFileContent (GET returns body, HEAD headers only,
//     Range returns 206, ETag + Content-Length set by the VFS layer).
//   - Collection: 405 with Allow: OPTIONS, PROPFIND, HEAD.
//   - Traversal / forbidden path: 403, audit-logged.
//   - Missing path: 404.
//   - Out-of-scope permission: 403, audit-logged.
func handleGet(c echo.Context) error {
	rawParam := c.Param("*")
	vfsPath, err := davPathToVFSPath(rawParam)
	if err != nil {
		auditLog(c, "get path rejected", rawParam)
		return sendWebDAVError(c, http.StatusForbidden, "forbidden")
	}

	inst := middlewares.GetInstance(c)
	dirDoc, fileDoc, err := inst.VFS().DirOrFileByPath(vfsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sendWebDAVError(c, http.StatusNotFound, "not-found")
		}
		return err
	}

	if dirDoc != nil {
		// GET/HEAD on a collection → 405 Method Not Allowed.
		// The Allow header advertises the methods that ARE valid on a
		// collection (OPTIONS discovery, PROPFIND metadata, HEAD no-op).
		c.Response().Header().Set("Allow", "OPTIONS, PROPFIND, HEAD")
		return sendWebDAVError(c, http.StatusMethodNotAllowed, "method-not-allowed")
	}

	if err := middlewares.AllowVFS(c, permission.GET, fileDoc); err != nil {
		auditLog(c, "get out-of-scope", vfsPath)
		return sendWebDAVError(c, http.StatusForbidden, "forbidden")
	}

	// Delegate to the VFS — it handles Range, ETag, Last-Modified,
	// Content-Length, and HEAD (via http.ServeContent) automatically.
	return vfs.ServeFileContent(inst.VFS(), fileDoc, nil, "", "", c.Request(), c.Response())
}
