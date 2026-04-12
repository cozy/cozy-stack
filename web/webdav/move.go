package webdav

import (
	"errors"
	"net/http"
	"os"
	"path"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// handleMove handles HTTP MOVE requests for renaming and reparenting files
// and directories. Per RFC 4918 section 9.9, MOVE renames the resource at
// the Request-URI to the location given in the Destination header.
//
// Contract:
//   - Rename or reparent to new path: 201 Created
//   - Overwrite:T (or absent, which defaults to T) with existing destination:
//     trash the destination first, then move source -> 204 No Content
//   - Overwrite:F with existing destination: 412 Precondition Failed
//   - Destination inside .cozy_trash: 403 Forbidden
//   - Missing Destination header: 400 Bad Request
//   - Destination parent does not exist: 409 Conflict
//
// Uses vfs.ModifyFileMetadata/ModifyDirMetadata with DocPatch{Name, DirID}
// for the actual rename/reparent. Overwrite:T trashes the existing
// destination via vfs.TrashFile/TrashDir (consistent with DELETE = soft-trash).
func handleMove(c echo.Context) error {
	// 1. Resolve source path.
	rawParam := c.Param("*")
	srcPath, err := davPathToVFSPath(rawParam)
	if err != nil {
		auditLog(c, "move source path rejected", rawParam)
		return sendWebDAVError(c, http.StatusForbidden, "forbidden")
	}

	// 2. Parse and validate Destination header.
	dstPath, err := parseDestination(c.Request())
	if err != nil {
		switch {
		case errors.Is(err, errMissingDestination):
			return sendWebDAVError(c, http.StatusBadRequest, "bad-request")
		case errors.Is(err, ErrPathTraversal):
			auditLog(c, "move destination traversal", c.Request().Header.Get("Destination"))
			return sendWebDAVError(c, http.StatusForbidden, "forbidden")
		default:
			// errInvalidDestination — wrong prefix or unparseable URL.
			// RFC 4918 section 9.9.4: cross-server destination -> 502.
			return sendWebDAVError(c, http.StatusBadGateway, "bad-gateway")
		}
	}

	// 3. Write-fence: reject MOVE into .cozy_trash.
	if isInTrash(dstPath) {
		auditLog(c, "move to trash attempt", dstPath)
		return sendWebDAVError(c, http.StatusForbidden, "forbidden")
	}

	inst := middlewares.GetInstance(c)
	fs := inst.VFS()

	// 4. Resolve the source resource.
	srcDir, srcFile, err := fs.DirOrFileByPath(srcPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sendWebDAVError(c, http.StatusNotFound, "not-found")
		}
		return mapVFSWriteError(c, err)
	}

	// 5. Determine Overwrite semantics.
	// RFC 4918 default is T (NOT F — avoids x/net/webdav bug #66059).
	overwrite := true
	if ovr := c.Request().Header.Get("Overwrite"); ovr == "F" {
		overwrite = false
	}

	// 6. Check if destination already exists.
	destExisted := false
	dstDir, dstFile, err := fs.DirOrFileByPath(dstPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return mapVFSWriteError(c, err)
	}
	if dstDir != nil || dstFile != nil {
		if !overwrite {
			return sendWebDAVError(c, http.StatusPreconditionFailed, "precondition-failed")
		}
		// Overwrite:T — trash the existing destination first.
		destExisted = true
		if dstFile != nil {
			_, err = vfs.TrashFile(fs, dstFile)
		} else {
			_, err = vfs.TrashDir(fs, dstDir)
		}
		if err != nil {
			return mapVFSWriteError(c, err)
		}
	}

	// 7. Resolve destination parent directory.
	dstParent, err := fs.DirByPath(path.Dir(dstPath))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sendWebDAVError(c, http.StatusConflict, "conflict")
		}
		return mapVFSWriteError(c, err)
	}

	// 8. Build DocPatch and execute rename/reparent.
	newName := path.Base(dstPath)
	newDirID := dstParent.ID()
	patch := &vfs.DocPatch{
		Name:  &newName,
		DirID: &newDirID,
	}

	if srcFile != nil {
		_, err = vfs.ModifyFileMetadata(fs, srcFile, patch)
	} else {
		_, err = vfs.ModifyDirMetadata(fs, srcDir, patch)
	}
	if err != nil {
		return mapVFSWriteError(c, err)
	}

	// Move dead (custom) properties from the old path to the new path so that
	// PROPFIND on the destination reflects any properties set on the source.
	// RFC 4918 §9.9.1 requires that the MOVE operation move associated dead
	// properties along with the resource.
	deadPropStore.movePropsForPath(inst.Domain, srcPath, dstPath)

	// 9. Return status: 204 if destination was overwritten, 201 if new.
	if destExisted {
		return c.NoContent(http.StatusNoContent)
	}
	return c.NoContent(http.StatusCreated)
}
