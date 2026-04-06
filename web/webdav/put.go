package webdav

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// handlePut handles HTTP PUT requests for file creation and overwrite.
//
// Contract:
//   - New file (no existing file at path): 201 Created
//   - Overwrite (existing file): 204 No Content
//   - Missing parent directory: 409 Conflict
//   - If-Match mismatch: 412 Precondition Failed
//   - If-None-Match: * on existing file: 412 Precondition Failed
//   - Path inside .cozy_trash: 403 Forbidden
//   - Zero-byte body: creates empty file (201)
//   - Quota exceeded: 507 Insufficient Storage (surfaces on file.Close())
func handlePut(c echo.Context) error {
	rawParam := c.Param("*")
	vfsPath, err := davPathToVFSPath(rawParam)
	if err != nil {
		auditLog(c, "put path rejected", rawParam)
		return sendWebDAVError(c, http.StatusForbidden, "forbidden")
	}

	// Write-fence: reject writes into .cozy_trash.
	if isInTrash(vfsPath) {
		auditLog(c, "put into trash rejected", vfsPath)
		return sendWebDAVError(c, http.StatusForbidden, "forbidden")
	}

	inst := middlewares.GetInstance(c)
	fs := inst.VFS()

	// Resolve the parent directory — must exist for both create and overwrite.
	parentPath := path.Dir(vfsPath)
	parent, err := fs.DirByPath(parentPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sendWebDAVError(c, http.StatusConflict, "conflict")
		}
		return err
	}

	// Check if the file already exists (overwrite vs create).
	_, existingFile, err := fs.DirOrFileByPath(vfsPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	isOverwrite := existingFile != nil

	// Conditional header checks (only meaningful on overwrite).
	if isOverwrite {
		if err := checkETagPreconditions(c, existingFile); err != nil {
			return sendWebDAVError(c, http.StatusPreconditionFailed, "precondition-failed")
		}
	} else {
		// For creates, If-None-Match: * is the only relevant conditional.
		// It succeeds (file does not exist) — no action needed.
		// But if someone sends If-Match on a non-existing file, that's also
		// fine — there's nothing to match against, and we just create.
	}

	// Determine Content-Type.
	filename := path.Base(vfsPath)
	mime, class := detectMimeAndClass(c, filename)

	// Content-Length: use request's ContentLength. A value of -1 (chunked)
	// is accepted by VFS and means "unknown size".
	size := c.Request().ContentLength
	if size < 0 {
		size = -1
	}

	newdoc, err := vfs.NewFileDoc(
		filename, parent.ID(), size, nil,
		mime, class, time.Now(),
		false, false, false, nil,
	)
	if err != nil {
		return mapVFSWriteError(c, err)
	}

	var olddoc *vfs.FileDoc
	if isOverwrite {
		olddoc = existingFile
	}

	file, err := fs.CreateFile(newdoc, olddoc)
	if err != nil {
		return mapVFSWriteError(c, err)
	}

	_, err = io.Copy(file, c.Request().Body)
	// CRITICAL: file.Close() commits the write to the VFS. Quota overflow
	// and content-length mismatch errors surface here, not during io.Copy.
	if cerr := file.Close(); cerr != nil && err == nil {
		err = cerr
	}
	if err != nil {
		return mapVFSWriteError(c, err)
	}

	if isOverwrite {
		return c.NoContent(http.StatusNoContent)
	}
	return c.NoContent(http.StatusCreated)
}

// detectMimeAndClass determines the MIME type and class for a new file.
// It trusts the client's Content-Type header if present and not the
// generic "application/octet-stream". Falls back to extension-based
// detection via vfs.ExtractMimeAndClassFromFilename.
func detectMimeAndClass(c echo.Context, filename string) (string, string) {
	ct := c.Request().Header.Get("Content-Type")
	if ct != "" && ct != "application/octet-stream" {
		mime, class := vfs.ExtractMimeAndClass(ct)
		return mime, class
	}
	return vfs.ExtractMimeAndClassFromFilename(filename)
}
