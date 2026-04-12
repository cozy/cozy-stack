package webdav

import (
	"errors"
	"net/http"
	"os"
	"path"

	"github.com/cozy/cozy-stack/model/note"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// handleCopy handles HTTP COPY requests for duplicating files and directories
// per RFC 4918 §9.8. Structurally a twin of handleMove — same parseDestination,
// same Overwrite semantics, same trash-then-write pattern. The VFS verb is
// fs.CopyFile (or note.CopyFile for olddoc.Mime == consts.NoteMimeType).
//
// Contract (file mode — directory mode lives in plan 03-02):
//   - New destination: 201 Created
//   - Overwrite:T (or absent) with existing destination: trash dest, copy source -> 204
//   - Overwrite:F with existing destination: 412 Precondition Failed
//   - Source == Destination (RFC 4918 §9.8.5): 403 Forbidden
//   - Source or Destination inside .cozy_trash: 403 Forbidden
//   - Missing Destination header: 400 Bad Request
//   - Destination parent missing: 409 Conflict
//   - Source Mime == consts.NoteMimeType: delegates to note.CopyFile
func handleCopy(c echo.Context) error {
	// 1. Resolve source path.
	rawParam := c.Param("*")
	srcPath, err := davPathToVFSPath(rawParam)
	if err != nil {
		auditLog(c, "copy source path rejected", rawParam)
		return sendWebDAVError(c, http.StatusForbidden, "forbidden")
	}

	// 2. Parse and validate Destination header.
	dstPath, err := parseDestination(c.Request())
	if err != nil {
		switch {
		case errors.Is(err, errMissingDestination):
			return sendWebDAVError(c, http.StatusBadRequest, "bad-request")
		case errors.Is(err, ErrPathTraversal):
			auditLog(c, "copy destination traversal", c.Request().Header.Get("Destination"))
			return sendWebDAVError(c, http.StatusForbidden, "forbidden")
		default:
			// errInvalidDestination — wrong prefix or unparseable URL.
			// RFC 4918 section 9.8.4: cross-server destination -> 502.
			return sendWebDAVError(c, http.StatusBadGateway, "bad-gateway")
		}
	}

	// 3. Write-fence: reject COPY from .cozy_trash (source guard — not in MOVE).
	if isInTrash(srcPath) {
		auditLog(c, "copy from trash attempt", srcPath)
		return sendWebDAVError(c, http.StatusForbidden, "forbidden")
	}

	// 4. Write-fence: reject COPY into .cozy_trash.
	if isInTrash(dstPath) {
		auditLog(c, "copy to trash attempt", dstPath)
		return sendWebDAVError(c, http.StatusForbidden, "forbidden")
	}

	// 5. Source == destination guard (RFC 4918 §9.8.5).
	if srcPath == dstPath {
		auditLog(c, "copy source equals destination", srcPath)
		return sendWebDAVError(c, http.StatusForbidden, "forbidden")
	}

	inst := middlewares.GetInstance(c)
	fs := inst.VFS()

	// 6. Resolve the source resource.
	srcDir, srcFile, err := fs.DirOrFileByPath(srcPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sendWebDAVError(c, http.StatusNotFound, "not-found")
		}
		return mapVFSWriteError(c, err)
	}

	// 7. File-only gate for this plan — directory COPY is plan 03-02.
	if srcDir != nil && srcFile == nil {
		// TODO(plan 03-02): implement recursive directory COPY via vfs.Walk.
		return sendWebDAVError(c, http.StatusNotImplemented, "not-implemented")
	}

	// 8. Determine Overwrite semantics.
	// RFC 4918 default is T (absent == T, per §10.6).
	overwrite := c.Request().Header.Get("Overwrite") != "F"

	// 9. Check if destination already exists.
	destExisted := false
	_, dstFile, err := fs.DirOrFileByPath(dstPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return mapVFSWriteError(c, err)
	}
	if dstFile != nil {
		if !overwrite {
			return sendWebDAVError(c, http.StatusPreconditionFailed, "precondition-failed")
		}
		// Overwrite:T — trash the existing destination first.
		destExisted = true
		if _, err = vfs.TrashFile(fs, dstFile); err != nil {
			return mapVFSWriteError(c, err)
		}
	}

	// 10. Resolve destination parent directory.
	dstParent, err := fs.DirByPath(path.Dir(dstPath))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sendWebDAVError(c, http.StatusConflict, "conflict")
		}
		return mapVFSWriteError(c, err)
	}

	// 11. Build the destination FileDoc.
	newName := path.Base(dstPath)
	newdoc := vfs.CreateFileDocCopy(srcFile, dstParent.ID(), newName)

	// 12. Branch on source mime type (pitfall A: use srcFile.Mime, not newdoc.Mime).
	// CreateFileDocCopy re-derives Mime from filename when copyName is non-empty,
	// so newdoc.Mime may differ from srcFile.Mime after the copy is built.
	if srcFile.Mime == consts.NoteMimeType {
		err = note.CopyFile(inst, srcFile, newdoc)
	} else {
		err = fs.CopyFile(srcFile, newdoc)
	}
	if err != nil {
		return mapVFSWriteError(c, err)
	}

	// 13. Return status: 204 if destination was overwritten, 201 if new.
	if destExisted {
		return c.NoContent(http.StatusNoContent)
	}
	return c.NoContent(http.StatusCreated)
}
