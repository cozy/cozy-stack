package webdav

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"

	"github.com/cozy/cozy-stack/model/instance"
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
// Contract (file mode):
//   - New destination: 201 Created
//   - Overwrite:T (or absent) with existing destination: trash dest, copy source -> 204
//   - Overwrite:F with existing destination: 412 Precondition Failed
//   - Source == Destination (RFC 4918 §9.8.5): 403 Forbidden
//   - Source or Destination inside .cozy_trash: 403 Forbidden
//   - Missing Destination header: 400 Bad Request
//   - Destination parent missing: 409 Conflict
//   - Source Mime == consts.NoteMimeType: delegates to note.CopyFile
//
// Contract (directory mode — RFC 4918 §9.8.3):
//   - Depth absent or "infinity": recursive copy of entire subtree
//   - Depth "0": create empty destination directory only
//   - Depth "1": 400 Bad Request (RFC 4918 forbids Depth:1 on COPY)
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

	// 7. Branch on source type: directory vs file.
	if srcDir != nil && srcFile == nil {
		return handleCopyDir(c, fs, inst, srcDir, srcPath, dstPath)
	}

	// 8. Determine Overwrite semantics.
	// RFC 4918 default is T (absent == T, per §10.6).
	overwrite := c.Request().Header.Get("Overwrite") != "F"

	// 9. Check if destination already exists (file or directory).
	// RFC 4918 §9.8.4: Overwrite:T replaces the destination regardless of its
	// type — a collection at the destination must be trashed just like a file.
	destExisted := false
	dstDir, dstFile, err := fs.DirOrFileByPath(dstPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return mapVFSWriteError(c, err)
	}
	if dstFile != nil || dstDir != nil {
		if !overwrite {
			return sendWebDAVError(c, http.StatusPreconditionFailed, "precondition-failed")
		}
		// Overwrite:T — trash the existing destination first.
		destExisted = true
		if dstFile != nil {
			if _, err = vfs.TrashFile(fs, dstFile); err != nil {
				return mapVFSWriteError(c, err)
			}
		} else {
			if _, err = vfs.TrashDir(fs, dstDir); err != nil {
				return mapVFSWriteError(c, err)
			}
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

// copyFailure records a per-member failure during a directory COPY walk.
// Used to build the 207 Multi-Status response body per RFC 4918 §9.8.7.
type copyFailure struct {
	hrefPath string
	status   int
}

// handleCopyDir implements recursive directory COPY per RFC 4918 §9.8.
//
// Depth semantics (RFC 4918 §9.8.3):
//   - absent or "infinity" → recursive copy of entire subtree (default)
//   - "0"                  → shallow copy: create empty destination directory
//   - "1"                  → 400 Bad Request (RFC 4918 forbids Depth:1 on COPY)
//
// Overwrite semantics mirror the file case: absent == T, "F" == 412, "T" ==
// trash the existing destination directory then copy.
//
// Per-member failures are collected (walk is NOT aborted) and reported in a
// 207 Multi-Status response per RFC 4918 §9.8.7. Successful members are left
// in place — no rollback on partial failure.
func handleCopyDir(c echo.Context, fs vfs.VFS, inst *instance.Instance, srcDir *vfs.DirDoc, srcPath, dstPath string) error {
	// RFC 4918 §9.8.3: Depth:1 is forbidden on collection COPY.
	depthHdr := c.Request().Header.Get("Depth")
	if depthHdr == "1" {
		return sendWebDAVError(c, http.StatusBadRequest, "bad-request")
	}
	recursive := depthHdr != "0"

	overwrite := c.Request().Header.Get("Overwrite") != "F"

	// Check if destination already exists (can be a dir or file).
	dstExisted := false
	dstDir, dstFile, err := fs.DirOrFileByPath(dstPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return mapVFSWriteError(c, err)
	}
	if dstFile != nil || dstDir != nil {
		if !overwrite {
			return sendWebDAVError(c, http.StatusPreconditionFailed, "precondition-failed")
		}
		dstExisted = true
		// Trash whatever is at the destination.
		if dstFile != nil {
			if _, err = vfs.TrashFile(fs, dstFile); err != nil {
				return mapVFSWriteError(c, err)
			}
		} else {
			if _, err = vfs.TrashDir(fs, dstDir); err != nil {
				return mapVFSWriteError(c, err)
			}
		}
	}

	// Resolve destination parent directory.
	dstParentPath := path.Dir(dstPath)
	dstParent, err := fs.DirByPath(dstParentPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sendWebDAVError(c, http.StatusConflict, "conflict")
		}
		return mapVFSWriteError(c, err)
	}

	// Create the root destination directory.
	dstName := path.Base(dstPath)
	dstRootDir, err := vfs.NewDirDocWithParent(dstName, dstParent, nil)
	if err != nil {
		return mapVFSWriteError(c, err)
	}
	if err = fs.CreateDir(dstRootDir); err != nil {
		return mapVFSWriteError(c, err)
	}

	if !recursive {
		// Depth:0 — empty directory copy done.
		if dstExisted {
			return c.NoContent(http.StatusNoContent)
		}
		return c.NoContent(http.StatusCreated)
	}

	// Recursive walk: dirMap tracks srcDirID → dstDir so we can wire each
	// child to its destination parent.
	//
	// Per RFC 4918 §9.8.7, errors on individual members MUST NOT abort the
	// walk — they are collected and reported in a 207 Multi-Status body.
	// Only catastrophic errors (ErrWalkOverflow, index unreachable) abort.
	var failures []copyFailure

	dirMap := map[string]*vfs.DirDoc{
		srcDir.DocID: dstRootDir,
	}

	walkErr := vfs.Walk(fs, srcPath, func(entryPath string, d *vfs.DirDoc, f *vfs.FileDoc, werr error) error {
		if werr != nil {
			// Walk-level error for this entry — collect and continue (pitfall C).
			failures = append(failures, copyFailure{hrefPath: entryPath, status: http.StatusInternalServerError})
			return nil
		}

		// Skip the root (already created above).
		if d != nil && d.DocID == srcDir.DocID {
			return nil
		}

		if d != nil {
			// Subdirectory: create a mirror under the already-created parent.
			parentDst, ok := dirMap[d.DirID]
			if !ok {
				// Malformed VFS tree — collect and continue.
				failures = append(failures, copyFailure{hrefPath: entryPath, status: http.StatusInternalServerError})
				return nil
			}
			newSubDir, mkErr := vfs.NewDirDocWithParent(d.DocName, parentDst, nil)
			if mkErr != nil {
				failures = append(failures, copyFailure{hrefPath: entryPath, status: httpStatusForVFSErr(mkErr)})
				return nil
			}
			if mkErr = fs.CreateDir(newSubDir); mkErr != nil {
				failures = append(failures, copyFailure{hrefPath: entryPath, status: httpStatusForVFSErr(mkErr)})
				return nil
			}
			dirMap[d.DocID] = newSubDir
			return nil
		}

		// File: copy to corresponding destination directory.
		parentDst, ok := dirMap[f.DirID]
		if !ok {
			failures = append(failures, copyFailure{hrefPath: entryPath, status: http.StatusConflict})
			return nil
		}
		newFileDoc := vfs.CreateFileDocCopy(f, parentDst.ID(), f.DocName)
		// Use f.Mime (srcFile.Mime) for Note branch — same pitfall A as file COPY.
		var copyErr error
		if f.Mime == consts.NoteMimeType {
			copyErr = note.CopyFile(inst, f, newFileDoc)
		} else {
			copyErr = fs.CopyFile(f, newFileDoc)
		}
		if copyErr != nil {
			failures = append(failures, copyFailure{hrefPath: entryPath, status: httpStatusForVFSErr(copyErr)})
		}
		return nil
	})
	if walkErr != nil {
		// Catastrophic walk error (ErrWalkOverflow or similar) — abort with 500.
		return mapVFSWriteError(c, walkErr)
	}

	// If any member failures occurred, return 207 Multi-Status per RFC 4918 §9.8.7.
	// Already-copied children stay in place (no rollback).
	if len(failures) > 0 {
		return sendCopyMultiStatus(c, failures)
	}

	if dstExisted {
		return c.NoContent(http.StatusNoContent)
	}
	return c.NoContent(http.StatusCreated)
}

// httpStatusForVFSErr maps a VFS error to an appropriate HTTP status code for
// use in 207 Multi-Status per-response entries.
func httpStatusForVFSErr(err error) int {
	switch {
	case errors.Is(err, vfs.ErrFileTooBig), errors.Is(err, vfs.ErrMaxFileSize):
		return http.StatusInsufficientStorage // 507
	case errors.Is(err, os.ErrExist):
		return http.StatusPreconditionFailed // 412
	case errors.Is(err, os.ErrNotExist):
		return http.StatusNotFound // 404
	default:
		return http.StatusInternalServerError // 500
	}
}

// sendCopyMultiStatus writes a 207 Multi-Status body listing all per-member
// failures that occurred during a directory COPY walk. Successful members are
// NOT listed (RFC 4918 §9.8 — only failures appear).
//
// The root element is written manually as `<D:multistatus xmlns:D="DAV:">` per
// the Phase 1 XML convention (encoding/xml.Marshal leaks xmlns="DAV:" on every
// child — see propfind.go for the established pattern).
func sendCopyMultiStatus(c echo.Context, failures []copyFailure) error {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	buf.WriteString(`<D:multistatus xmlns:D="DAV:">` + "\n")
	for _, f := range failures {
		buf.WriteString("  <D:response>\n")
		buf.WriteString("    <D:href>")
		_ = xml.EscapeText(&buf, []byte(f.hrefPath))
		buf.WriteString("</D:href>\n")
		buf.WriteString(fmt.Sprintf("    <D:status>HTTP/1.1 %d %s</D:status>\n",
			f.status, http.StatusText(f.status)))
		buf.WriteString("  </D:response>\n")
	}
	buf.WriteString(`</D:multistatus>` + "\n")

	h := c.Response().Header()
	h.Set(echo.HeaderContentType, "application/xml; charset=utf-8")
	h.Set(echo.HeaderContentLength, strconv.Itoa(buf.Len()))
	return c.Blob(http.StatusMultiStatus, "application/xml; charset=utf-8", buf.Bytes())
}
