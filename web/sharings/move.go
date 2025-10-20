package sharings

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/permission"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

const (
	jsonAPIContentType = "application/vnd.api+json"
)

type moveRequest struct {
	Source struct {
		Instance  string `json:"instance"`
		SharingID string `json:"sharing_id"`
		FileID    string `json:"file_id"`
		DirID     string `json:"dir_id"`
	} `json:"source"`
	Dest struct {
		Instance  string `json:"instance"`
		SharingID string `json:"sharing_id"`
		DirID     string `json:"dir_id"`
	} `json:"dest"`
	Copy bool `json:"copy,omitempty"`
}

// validateMoveRequest checks the structural validity of a move request and
// returns a 400-style error via jsonapi when invalid.
func validateMoveRequest(req moveRequest) error {
	if req.Source.FileID == "" && req.Source.DirID == "" {
		return jsonapi.BadRequest(errors.New("missing source file_id and dir_id"))
	}
	if req.Source.FileID != "" && req.Source.DirID != "" {
		return jsonapi.BadRequest(errors.New("ambiguous file_id and dir_id are set, please specify only one of them"))
	}
	if req.Dest.DirID == "" {
		return jsonapi.BadRequest(errors.New("missing dest dir_id"))
	}

	hasSourceInstance := req.Source.Instance != ""
	hasDestInstance := req.Dest.Instance != ""
	hasSourceSharing := req.Source.SharingID != ""
	hasDestSharing := req.Dest.SharingID != ""

	if hasSourceInstance && !hasSourceSharing {
		return jsonapi.BadRequest(errors.New("source sharing_id is required when source instance is provided"))
	}
	if hasDestInstance && !hasDestSharing {
		return jsonapi.BadRequest(errors.New("dest sharing_id is required when dest instance is provided"))
	}
	if !hasSourceSharing && !hasDestSharing {
		return jsonapi.BadRequest(errors.New("at least one sharing_id must be provided"))
	}

	return nil
}

// MoveHandler is a unified endpoint to move a file or directory between.
// Accepted inputs (JSON body preferred, query params supported for compatibility):
// - Common: file-id, dir-id
// - Source: source-instance + source-sharing_id (both together for shared drives)
// - Destination: dest-instance + dest-sharing_id (both together for shared drives)
// - Copy: boolean flag (default false) - when true, performs copy instead of move (does not delete source)
// For personal drives, only sharing_id is provided (instance is empty).
// At least one sharing_id must be provided.
//
// Also supports a nested JSON structure with source and target objects:
//
//		{
//		  "source": {
//		    "instance": "https://alice.localhost:8080",
//		    "file_id": "file123",
//		    "sharing_id": "sharing123",
//		    "dir_id": "dir456"
//		  },
//		  "dest": {
//		    "instance": "https://alice.localhost:8080",
//	        "sharing_id": "sharing123",
//		    "dir_id": "dir456"
//		  },
//		  "copy": false
//		}
func MoveHandler(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	var req moveRequest
	if err := c.Bind(&req); err != nil {
		return jsonapi.BadRequest(errors.New("error unmarshalling request"))
	}
	if err := validateMoveRequest(req); err != nil {
		return err
	}

	// Authorization: check that we have permissions to the current instance, require whole-type permission
	if err := middlewares.AllowWholeType(c, permission.POST, consts.Files); err != nil {
		return err
	}

	sourceInstance, err := resolveInstanceOrCurrent(req.Source.Instance, inst)
	if err != nil {
		return err
	}
	destInstance, err := resolveInstanceOrCurrent(req.Dest.Instance, inst)
	if err != nil {
		return err
	}

	moveDirectory := req.Source.DirID != ""

	if req.Source.Instance != "" && req.Dest.Instance != "" {
		sourceSharing, err := checkSharedDrivePermission(inst, req.Source.SharingID)
		if err != nil {
			return err
		}
		destSharing, err := checkSharedDrivePermission(inst, req.Dest.SharingID)
		if err != nil {
			return err
		}
		if OnSameStackCheck(sourceInstance, destInstance) {
			if moveDirectory {
				return moveDirSameStack(c, sourceInstance, destInstance, req.Source.DirID, req.Dest.DirID, destSharing, req.Copy)
			} else {
				return moveFileSameStack(c, sourceInstance, destInstance, req.Source.FileID, req.Dest.DirID, destSharing, req.Copy)
			}
		} else {
			if moveDirectory {
				return moveDirBetweenSharedDrives(c, req.Source.Instance, req.Source.DirID, sourceSharing, req.Dest.Instance, req.Dest.DirID, destSharing, req.Copy)
			}
			return moveFileBetweenSharedDrives(c, req.Source.Instance, req.Source.FileID, sourceSharing, req.Dest.Instance, req.Dest.DirID, destSharing, req.Copy)
		}
	} else if req.Source.Instance != "" && req.Dest.Instance == "" {
		s, err := checkSharedDrivePermission(inst, req.Source.SharingID)
		if err != nil {
			return err
		}
		if OnSameStackCheck(sourceInstance, destInstance) {
			if moveDirectory {
				return moveDirSameStack(c, sourceInstance, destInstance, req.Source.DirID, req.Dest.DirID, nil, req.Copy)
			} else {
				return moveFileSameStack(c, sourceInstance, destInstance, req.Source.FileID, req.Dest.DirID, nil, req.Copy)
			}
		} else {
			if moveDirectory {
				return moveDirFromSharedDrive(c, destInstance, req.Source.Instance, req.Source.DirID, req.Dest.DirID, s, req.Copy)
			}
			return moveFileFromSharedDrive(c, destInstance, req.Source.Instance, req.Source.FileID, req.Dest.DirID, s, req.Copy)
		}
	} else if req.Source.Instance == "" && req.Dest.Instance != "" {
		s, err := checkSharedDrivePermission(inst, req.Dest.SharingID)
		if err != nil {
			return err
		}
		if OnSameStackCheck(sourceInstance, destInstance) {
			if moveDirectory {
				return moveDirSameStack(c, sourceInstance, destInstance, req.Source.DirID, req.Dest.DirID, s, req.Copy)
			} else {
				return moveFileSameStack(c, sourceInstance, destInstance, req.Source.FileID, req.Dest.DirID, s, req.Copy)
			}
		} else {
			if moveDirectory {
				return moveDirToSharedDrive(c, sourceInstance, req.Source.DirID, req.Dest.DirID, s, req.Copy)
			}
			return moveFileToSharedDrive(c, sourceInstance, req.Dest.DirID, req.Source.FileID, s, req.Copy)
		}
	} else {
		return jsonapi.BadRequest(errors.New("to move files inside personal drive use patch function"))
	}
}

// Same-stack moves (instance ↔ instance)
func moveDirSameStack(c echo.Context, srcInst *instance.Instance, destInst *instance.Instance, sourceDirID string,
	destDirID string, destSharing *sharing.Sharing, copy bool) error {
	// Resolve source and destination root directories
	srcRoot, err := srcInst.VFS().DirByID(sourceDirID)
	if err != nil {
		return files.WrapVfsError(err)
	}
	_, err = destInst.VFS().DirByID(destDirID)
	if err != nil {
		return files.WrapVfsError(err)
	}

	dirs, filesToMove, err := planLocalTree(srcInst.VFS(), srcRoot, destDirID)
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Map from source dirID to destination dirID
	srcToDstDirID := make(map[string]string)

	// Create all directories in a single pass, top-down order
	for i, d := range dirs {
		var parentDestID string
		if i == 0 {
			parentDestID = destDirID
		} else {
			var ok bool
			parentDestID, ok = srcToDstDirID[d.DirID]
			if !ok {
				return files.WrapVfsError(errors.New("parent directory mapping missing while creating destination directory"))
			}
		}
		// Ensure unique name for directory
		dirName, err := ensureUniqueName(destInst.VFS(), parentDestID, d.DocName, false)
		if err != nil {
			return files.WrapVfsError(err)
		}
		newDir, err := vfs.NewDirDoc(destInst.VFS(), dirName, parentDestID, d.Tags)
		if err != nil {
			return files.WrapVfsError(err)
		}
		if err := createLocalDir(destInst.VFS(), newDir); err != nil {
			return files.WrapVfsError(err)
		}
		srcToDstDirID[d.DocID] = newDir.DocID
	}

	// Get the created root directory for the response
	newRoot, err := destInst.VFS().DirByID(srcToDstDirID[srcRoot.DocID])
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Copy files and delete sources
	for _, f := range filesToMove {
		destParentID, ok := srcToDstDirID[f.DirID]
		if !ok {
			return files.WrapVfsError(errors.New("destination parent directory not found while moving file"))
		}
		// When moving a directory we copy files but do not delete sources here
		if _, err := moveFileSameStackCore(srcInst, destInst, f, destParentID, false); err != nil {
			return err
		}
	}

	// Delete source directories bottom-up (reverse order) only if not copying
	if !copy {
		_, _, err = srcInst.VFS().DeleteDirDocAndContent(srcRoot, false)
		if err != nil {
			return files.WrapVfsError(err)
		}
	}

	obj := files.NewDir(newRoot, destSharing)
	return jsonapi.Data(c, http.StatusCreated, obj, nil)
}

// moveFileSameStackCore performs the actual file move between two instances on the same stack.
// It returns the newly created destination file document.
func moveFileSameStackCore(srcInst *instance.Instance, destInst *instance.Instance, srcFile *vfs.FileDoc, destDirID string, deleteSource bool) (*vfs.FileDoc, error) {
	newFileDoc, err := createFileDocFromSource(srcFile, destDirID)
	if err != nil {
		return nil, files.WrapVfsError(err)
	}

	// Check for name conflicts and resolve automatically
	oldName := newFileDoc.DocName
	uniqueName, err := ensureUniqueName(destInst.VFS(), destDirID, oldName, true)
	if err != nil {
		return nil, files.WrapVfsError(err)
	}
	if uniqueName != oldName {
		newFileDoc.DocName = uniqueName
		newFileDoc.ResetFullpath()
	}

	if err := copyFileContent(destInst.VFS(), newFileDoc, srcInst.VFS(), srcFile); err != nil {
		return nil, err
	}
	if deleteSource {
		if err := deleteSourceFile(srcInst.VFS(), srcFile); err != nil {
			return nil, err
		}
	}
	return newFileDoc, nil
}

func moveFileSameStack(c echo.Context, srcInst *instance.Instance, destInst *instance.Instance, fileID string, dirID string, destSharing *sharing.Sharing, copy bool) error {
	srcFile, err := srcInst.VFS().FileByID(fileID)
	if err != nil {
		return files.WrapVfsError(err)
	}
	deleteSource := !copy
	newFileDoc, err := moveFileSameStackCore(srcInst, destInst, srcFile, dirID, deleteSource)
	if err != nil {
		return err
	}
	obj := files.NewFile(newFileDoc, destInst, destSharing)
	return jsonapi.Data(c, http.StatusCreated, obj, nil)
}

func moveFileFromSharedDrive(c echo.Context, inst *instance.Instance, sourceInstanceURL string, fileID string,
	dirID string, s *sharing.Sharing, copy bool) error {
	deleteSource := !copy
	newFileDoc, err := moveFileFromSharedDriveCore(inst, sourceInstanceURL, fileID, dirID, s, deleteSource)
	if err != nil {
		return err
	}
	obj := files.NewFile(newFileDoc, inst, nil)
	return jsonapi.Data(c, http.StatusCreated, obj, nil)
}

// moveFileFromSharedDriveCore performs the actual file move from a remote shared drive
// to the local instance. It returns the created destination file document.
func moveFileFromSharedDriveCore(inst *instance.Instance, sourceInstanceURL string, fileID string,
	dirID string, s *sharing.Sharing, delete bool) (*vfs.FileDoc, error) {
	destDir, err := inst.VFS().DirByID(dirID)
	if err != nil {
		return nil, files.WrapVfsError(err)
	}
	u, err := url.Parse(sourceInstanceURL)
	if err != nil {
		return nil, files.WrapVfsError(err)
	}

	bearer, err := extractBearerToken(s)
	if err != nil {
		return nil, err
	}
	srcClient := NewRemoteClient(u, bearer)

	srcFile, err := srcClient.GetFileByID(fileID)
	if err != nil {
		return nil, files.WrapVfsError(err)
	}

	newFileDoc, err := createFileDocFromRemoteFile(srcFile, destDir.DocID)
	if err != nil {
		return nil, files.WrapVfsError(err)
	}

	// Check for name conflicts and resolve automatically
	oldName := newFileDoc.DocName
	uniqueName, err := ensureUniqueName(inst.VFS(), dirID, oldName, true)
	if err != nil {
		return nil, files.WrapVfsError(err)
	}
	if uniqueName != oldName {
		newFileDoc.DocName = uniqueName
		newFileDoc.ResetFullpath()
	}

	fd, err := inst.VFS().CreateFile(newFileDoc, nil)
	if err != nil {
		return nil, files.WrapVfsError(err)
	}

	rc, err := srcClient.DownloadByID(fileID)
	if err != nil {
		// Best-effort close to avoid leaking descriptors on error paths
		_ = fd.Close()
		return nil, files.WrapVfsError(err)
	}
	defer rc.Close()

	if _, err := io.Copy(fd, rc); err != nil {
		// Best-effort close to avoid leaking descriptors on error paths
		_ = fd.Close()
		return nil, files.WrapVfsError(err)
	}
	if err := fd.Close(); err != nil {
		return nil, err
	}
	if delete {
		if err := srcClient.PermanentDeleteByID(fileID); err != nil {
			inst.Logger().WithNamespace("move").Warnf("Could not delete source file: %v", err)
		}
	}
	return newFileDoc, nil
}

func moveDirFromSharedDrive(c echo.Context, inst *instance.Instance, sourceInstanceURL string, sourceDirID string,
	targetDirID string, s *sharing.Sharing, copy bool) error {
	_, err := inst.VFS().DirByID(targetDirID)
	if err != nil {
		return files.WrapVfsError(err)
	}
	u, err := url.Parse(sourceInstanceURL)
	if err != nil {
		return files.WrapVfsError(err)
	}

	bearer, err := extractBearerToken(s)
	if err != nil {
		return err
	}
	srcClient := NewRemoteClient(u, bearer)

	// Get the remote directory structure
	dirs, filesToMove, err := remoteContentToMove(srcClient, sourceDirID)
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Map from source dirID to destination dirID
	srcToDstDirID := make(map[string]string)

	// Create all directories in a single pass, top-down order
	for i, d := range dirs {
		var parentDestID string
		if i == 0 {
			parentDestID = targetDirID
		} else {
			var ok bool
			parentDestID, ok = srcToDstDirID[d.Attrs.DirID]
			if !ok {
				return files.WrapVfsError(errors.New("parent directory mapping missing while creating destination directory"))
			}
		}
		// Check for name conflicts and resolve automatically
		dirName, err := ensureUniqueName(inst.VFS(), parentDestID, d.Attrs.Name, false)
		if err != nil {
			return files.WrapVfsError(err)
		}

		newDir, err := vfs.NewDirDoc(inst.VFS(), dirName, parentDestID, d.Attrs.Tags)
		if err != nil {
			return files.WrapVfsError(err)
		}
		if err := createLocalDir(inst.VFS(), newDir); err != nil {
			return files.WrapVfsError(err)
		}
		srcToDstDirID[d.ID] = newDir.DocID
	}

	// Copy files and delete sources
	for _, f := range filesToMove {
		destParentID, ok := srcToDstDirID[f.Attrs.DirID]
		if !ok {
			return files.WrapVfsError(errors.New("destination parent directory not found while moving file"))
		}
		// Reuse single-file move core to handle copy + conflicts + deletion
		if _, err := moveFileFromSharedDriveCore(inst, sourceInstanceURL, f.ID, destParentID, s, false); err != nil {
			return err
		}
	}

	// Delete source directories bottom-up (reverse order) only if not copying
	if !copy {
		if err := srcClient.PermanentDeleteByID(sourceDirID); err != nil {
			inst.Logger().WithNamespace("move").Warnf("Could not delete source directory: %v", err)
		}
	}

	// Get the created root directory for the response
	newRoot, err := inst.VFS().DirByID(srcToDstDirID[sourceDirID])
	if err != nil {
		return files.WrapVfsError(err)
	}

	obj := files.NewDir(newRoot, nil)
	return jsonapi.Data(c, http.StatusCreated, obj, nil)
}

// Local → Shared-drive moves
func moveDirToSharedDrive(c echo.Context, srcInst *instance.Instance,
	sourceDirID string, destDirID string, s *sharing.Sharing, copy bool) error {
	srcRoot, err := srcInst.VFS().DirByID(sourceDirID)
	if err != nil {
		return files.WrapVfsError(err)
	}
	dstClient, err := NewSharedDriveClient(s)
	if err != nil {
		return err
	}

	dirs, filesToMove, err := planLocalTree(srcInst.VFS(), srcRoot, destDirID)
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Map from source dirID to destination dirID
	srcToDstDirID := make(map[string]string)

	// Create all directories in a single pass, top-down order
	for i, d := range dirs {
		var parentDestID string
		if i == 0 {
			parentDestID = destDirID
		} else {
			var ok bool
			parentDestID, ok = srcToDstDirID[d.DirID]
			if !ok {
				return files.WrapVfsError(errors.New("parent directory mapping missing while creating destination directory"))
			}
		}
		dstDir, err := dstClient.GetDirByID(parentDestID)
		if err != nil {
			return files.WrapVfsError(err)
		}
		newID, err := ensureRemoteChildDir(dstClient, dstDir, d.DocName)
		if err != nil {
			return files.WrapVfsError(err)
		}
		srcToDstDirID[d.DocID] = newID
	}

	// Copy files and delete sources
	for _, f := range filesToMove {
		destParentID, ok := srcToDstDirID[f.DirID]
		if !ok {
			return files.WrapVfsError(errors.New("destination parent directory not found while moving file"))
		}
		// When moving a directory we copy files but do not delete sources here
		if _, err := moveFileToSharedDriveCore(srcInst, destParentID, f.DocID, s, false); err != nil {
			return files.WrapVfsError(err)
		}
	}

	// Delete source directories bottom-up (reverse order) only if not copying
	if !copy {
		_, _, err = srcInst.VFS().DeleteDirDocAndContent(srcRoot, false)
		if err != nil {
			return files.WrapVfsError(err)
		}
	}

	dstDir, err := dstClient.GetDirByID(destDirID)
	if err != nil {
		return files.WrapVfsError(err)
	}
	movedDir, err := dstClient.GetDirByPath(dstDir.Attrs.Fullpath + "/" + srcRoot.DocName)
	if err != nil {
		return files.WrapVfsError(err)
	}

	return respondRemoteUploadDir(c, movedDir, s.ID())
}

func moveFileToSharedDrive(c echo.Context, inst *instance.Instance,
	targetDirID string, sourceFileID string, s *sharing.Sharing, copy bool) error {
	deleteSource := !copy
	uploaded, err := moveFileToSharedDriveCore(inst, targetDirID, sourceFileID, s, deleteSource)
	if err != nil {
		return err
	}
	return respondRemoteUpload(c, uploaded, s.ID())
}

func moveFileToSharedDriveCore(inst *instance.Instance,
	targetDirID string, sourceFileID string, s *sharing.Sharing, delete bool) (*client.File, error) {
	localSrcFile, err := inst.VFS().FileByID(sourceFileID)
	if err != nil {
		return nil, files.WrapVfsError(err)
	}
	dstClient, err := NewSharedDriveClient(s)
	if err != nil {
		return nil, err
	}

	srcHandle, err := inst.VFS().OpenFile(localSrcFile)
	if err != nil {
		return nil, files.WrapVfsError(err)
	}
	defer srcHandle.Close()

	// Optimistic upload with conflict retry
	dstParent, err := dstClient.GetDirByID(targetDirID)
	if err != nil {
		return nil, files.WrapVfsError(err)
	}
	uploaded, err := uploadWithConflictRetry(dstClient, dstParent.Attrs.Fullpath, targetDirID, localSrcFile.DocName, localSrcFile.MD5Sum, srcHandle, localSrcFile.Mime, localSrcFile.ByteSize)
	if err != nil {
		return nil, files.WrapVfsError(err)
	}

	if delete {
		if err := deleteSourceFile(inst.VFS(), localSrcFile); err != nil {
			return nil, err
		}
	}

	return uploaded, nil
}

func remoteContentToMove(remoteClient *client.Client, dirID string) ([]*client.DirOrFile, []*client.DirOrFile, error) {
	var dirs []*client.DirOrFile
	var filesToMove []*client.DirOrFile

	// Get the root directory first
	rootDir, err := remoteClient.GetDirByID(dirID)
	if err != nil {
		return nil, nil, err
	}

	// Add the root directory first to ensure it's created before subdirectories
	rootDoc := &client.DirOrFile{ID: rootDir.ID, Rev: rootDir.Rev}
	rootDoc.Attrs.Type = "directory"
	rootDoc.Attrs.Name = rootDir.Attrs.Name
	rootDoc.Attrs.DirID = rootDir.Attrs.DirID
	rootDoc.Attrs.CreatedAt = rootDir.Attrs.CreatedAt
	rootDoc.Attrs.UpdatedAt = rootDir.Attrs.UpdatedAt
	rootDoc.Attrs.Tags = rootDir.Attrs.Tags
	rootDoc.Attrs.Metadata = rootDir.Attrs.Metadata
	dirs = append(dirs, rootDoc)

	// Walk through the remote directory recursively
	err = remoteClient.WalkByPath(rootDir.Attrs.Fullpath, func(name string, doc *client.DirOrFile, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Skip the root directory itself as it's already added
		if doc.ID == rootDir.ID {
			return nil
		}

		if doc.Attrs.Type == "directory" {
			dirs = append(dirs, doc)
		} else if doc.Attrs.Type == "file" {
			filesToMove = append(filesToMove, doc)
		}

		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	return dirs, filesToMove, nil
}

func moveDirBetweenSharedDrives(c echo.Context, sourceInstanceURL, sourceDirID string, sourceSharing *sharing.Sharing, destInstanceURL, dirID string, destSharing *sharing.Sharing, copy bool) error {
	inst := middlewares.GetInstance(c)

	// Build remote clients for source and destination
	sourceURL, err := url.Parse(sourceInstanceURL)
	if err != nil {
		return files.WrapVfsError(err)
	}
	sourceBearer, err := extractBearerToken(sourceSharing)
	if err != nil {
		return err
	}
	sourceClient := NewRemoteClient(sourceURL, sourceBearer)

	destURL, err := url.Parse(destInstanceURL)
	if err != nil {
		return files.WrapVfsError(err)
	}
	destBearer, err := extractBearerToken(destSharing)
	if err != nil {
		return err
	}
	destClient := NewRemoteClient(destURL, destBearer)

	// Get the remote directory structure to move
	dirs, filesToMove, err := remoteContentToMove(sourceClient, sourceDirID)
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Map from source dirID to destination dirID
	srcToDstDirID := make(map[string]string)

	// Create all directories in destination, top-down
	for i, d := range dirs {
		var parentDestID string
		if i == 0 {
			parentDestID = dirID
		} else {
			var ok bool
			parentDestID, ok = srcToDstDirID[d.Attrs.DirID]
			if !ok {
				return files.WrapVfsError(errors.New("parent directory mapping missing while creating destination directory"))
			}
		}

		// Resolve parent path then create the directory remotely
		dstParent, err := destClient.GetDirByID(parentDestID)
		if err != nil {
			return files.WrapVfsError(err)
		}
		newID, err := ensureRemoteChildDir(destClient, dstParent, d.Attrs.Name)
		if err != nil {
			return files.WrapVfsError(err)
		}
		srcToDstDirID[d.ID] = newID
	}

	// Copy files from source remote to destination remote, then delete on source
	for _, f := range filesToMove {
		destParentID, ok := srcToDstDirID[f.Attrs.DirID]
		if !ok {
			return files.WrapVfsError(errors.New("destination parent directory not found while moving file"))
		}

		// Fetch source file metadata and content
		srcFile, err := sourceClient.GetFileByID(f.ID)
		if err != nil {
			return files.WrapVfsError(err)
		}
		srcReader, err := sourceClient.DownloadByID(f.ID)
		if err != nil {
			return files.WrapVfsError(err)
		}
		defer srcReader.Close()
		// Optimistic upload with conflict retry
		dstParent, err := destClient.GetDirByID(destParentID)
		if err != nil {
			return files.WrapVfsError(err)
		}
		_, err = uploadWithConflictRetry(destClient, dstParent.Attrs.Fullpath, destParentID, srcFile.Attrs.Name, srcFile.Attrs.MD5Sum, srcReader, srcFile.Attrs.Mime, srcFile.Attrs.Size)
		if err != nil {
			return files.WrapVfsError(err)
		}

		if !copy {
			if err := sourceClient.PermanentDeleteByID(f.ID); err != nil {
				inst.Logger().WithNamespace("move").Warnf("Could not delete source file: %v", err)
			}
		}
	}

	// Delete source directory (remote) after files are moved only if not copying
	if !copy {
		if err := sourceClient.PermanentDeleteByID(sourceDirID); err != nil {
			inst.Logger().WithNamespace("move").Warnf("Could not delete source directory: %v", err)
		}
	}

	// Respond with the created root directory on destination
	newRootID := srcToDstDirID[sourceDirID]
	dstRootDir, err := destClient.GetDirByID(newRootID)
	if err != nil {
		return files.WrapVfsError(err)
	}
	return respondRemoteUploadDir(c, dstRootDir, destSharing.ID())
}

func moveFileBetweenSharedDrives(c echo.Context, sourceInstanceURL, fileID string, sourceSharing *sharing.Sharing, destInstanceURL, destDirID string, destSharing *sharing.Sharing, copy bool) error {
	inst := middlewares.GetInstance(c)
	sourceURL, err := url.Parse(sourceInstanceURL)
	if err != nil {
		return files.WrapVfsError(err)
	}
	sourceBearer, err := extractBearerToken(sourceSharing)
	if err != nil {
		return err
	}
	sourceClient := NewRemoteClient(sourceURL, sourceBearer)

	destURL, err := url.Parse(destInstanceURL)
	if err != nil {
		return files.WrapVfsError(err)
	}
	destBearer, err := extractBearerToken(destSharing)
	if err != nil {
		return err
	}
	destClient := NewRemoteClient(destURL, destBearer)

	srcFile, err := sourceClient.GetFileByID(fileID)
	if err != nil {
		return files.WrapVfsError(err)
	}
	srcReader, err := sourceClient.DownloadByID(fileID)
	if err != nil {
		return files.WrapVfsError(err)
	}
	defer srcReader.Close()

	// Resolve destination parent fullpath for conflict checks
	dstParent, err := destClient.GetDirByID(destDirID)
	if err != nil {
		return files.WrapVfsError(err)
	}
	uniqueName, err := ensureRemoteUniqueChildName(destClient, dstParent.Attrs.Fullpath, srcFile.Attrs.Name, true)
	if err != nil {
		return err
	}
	uploaded, err := destClient.Upload(&client.Upload{
		Name:          uniqueName,
		DirID:         destDirID,
		ContentMD5:    srcFile.Attrs.MD5Sum,
		Contents:      srcReader,
		ContentType:   srcFile.Attrs.Mime,
		ContentLength: srcFile.Attrs.Size,
	})
	if err != nil {
		return files.WrapVfsError(err)
	}

	if !copy {
		if err := sourceClient.PermanentDeleteByID(fileID); err != nil {
			inst.Logger().WithNamespace("move").Warnf("Could not delete source file: %v", err)
		}
	}

	return respondRemoteUpload(c, uploaded, destSharing.ID())
}

func getInstanceIdentifierFromURL(urlStr string) (*instance.Instance, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	// Try to get the real instance first (for same-stack operations)
	inst, err := lifecycle.GetInstance(u.Host)
	if err == nil {
		return inst, nil
	}
	// If not found locally, return a minimal instance (for cross-stack operations)
	return &instance.Instance{Domain: u.Host}, nil
}

// resolveInstanceOrCurrent returns the instance resolved from the given URL,
// or the provided current instance when the URL is empty. Invalid URLs are
// reported as a BadRequest error suitable for handlers.
func resolveInstanceOrCurrent(urlStr string, current *instance.Instance) (*instance.Instance, error) {
	if urlStr == "" {
		return current, nil
	}
	inst, err := getInstanceIdentifierFromURL(urlStr)
	if err != nil {
		return nil, jsonapi.BadRequest(errors.New("invalid instance URL"))
	}
	return inst, nil
}

func onSameStack(src, dst *instance.Instance) bool {
	var srcPort, dstPort string
	parts := strings.SplitN(src.Domain, ":", 2)
	if len(parts) > 1 {
		srcPort = parts[1]
	}
	parts = strings.SplitN(dst.Domain, ":", 2)
	if len(parts) > 1 {
		dstPort = parts[1]
	}
	return srcPort == dstPort
}

// OnSameStackCheck allows tests to replace same-stack detection logic.
// Default points to onSameStack; tests may replace it.
var OnSameStackCheck = onSameStack

// NewRemoteClient allows tests to inject custom client behavior for cross-stack operations.
var NewRemoteClient = func(u *url.URL, bearerToken string) *client.Client {
	c := &client.Client{
		Scheme: u.Scheme,
		Addr:   u.Host,
		Domain: u.Hostname(),
	}
	if bearerToken != "" {
		c.Authorizer = &request.BearerAuthorizer{Token: bearerToken}
	}
	return c
}

var NewSharedDriveClient = func(s *sharing.Sharing) (*client.Client, error) {
	if len(s.Members) == 0 || s.Members[0].Instance == "" {
		return nil, jsonapi.Forbidden(errors.New("invalid sharing: missing member instance"))
	}
	destURL, err := url.Parse(s.Members[0].Instance)
	if err != nil {
		return nil, files.WrapVfsError(err)
	}
	bearer, err := extractBearerToken(s)
	if err != nil {
		return nil, files.WrapVfsError(err)
	}
	return NewRemoteClient(destURL, bearer), nil
}

// respondRemoteUpload returns a minimal JSONAPI-like response for a file created
// via remote upload (cross-stack path), matching existing attributes used by clients.
func respondRemoteUpload(c echo.Context, uploaded *client.File, driveId string) error {
	c.Response().Header().Set("Content-Type", jsonAPIContentType)
	attrs := map[string]interface{}{
		"name":       uploaded.Attrs.Name,
		"dir_id":     uploaded.Attrs.DirID,
		"type":       uploaded.Attrs.Type,
		"size":       uploaded.Attrs.Size,
		"mime":       uploaded.Attrs.Mime,
		"class":      uploaded.Attrs.Class,
		"executable": uploaded.Attrs.Executable,
		"tags":       uploaded.Attrs.Tags,
	}
	if driveId != "" {
		attrs["driveId"] = driveId
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"data": map[string]interface{}{
			"type":       consts.Files,
			"id":         uploaded.ID,
			"attributes": attrs,
		},
	})
}

func respondRemoteUploadDir(c echo.Context, uploaded *client.Dir, driveId string) error {
	c.Response().Header().Set("Content-Type", jsonAPIContentType)
	attrs := map[string]interface{}{
		"name":   uploaded.Attrs.Name,
		"dir_id": uploaded.Attrs.DirID,
		"type":   uploaded.Attrs.Type,
		"tags":   uploaded.Attrs.Tags,
	}
	if driveId != "" {
		attrs["driveId"] = driveId
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"data": map[string]interface{}{
			"type":       consts.Files,
			"id":         uploaded.ID,
			"attributes": attrs,
		},
	})
}

// Helpers to build new file docs and copy/delete content
func createFileDocFromSource(srcFile *vfs.FileDoc, targetDirID string) (*vfs.FileDoc, error) {
	return vfs.NewFileDoc(
		srcFile.DocName,
		targetDirID,
		srcFile.ByteSize,
		srcFile.MD5Sum,
		srcFile.Mime,
		srcFile.Class,
		time.Now(),
		srcFile.Executable,
		false,
		false,
		srcFile.Tags,
	)
}

func createFileDocFromRemoteFile(remoteFile *client.File, targetDirID string) (*vfs.FileDoc, error) {
	return vfs.NewFileDoc(
		remoteFile.Attrs.Name,
		targetDirID,
		remoteFile.Attrs.Size,
		remoteFile.Attrs.MD5Sum,
		remoteFile.Attrs.Mime,
		remoteFile.Attrs.Class,
		time.Now(),
		remoteFile.Attrs.Executable,
		false,
		remoteFile.Attrs.Encrypted,
		remoteFile.Attrs.Tags,
	)
}

func copyFileContent(destVFS vfs.VFS, newFileDoc *vfs.FileDoc, srcVFS vfs.VFS, srcFile *vfs.FileDoc) error {
	if err := destVFS.CopyFileFromOtherFS(newFileDoc, nil, srcVFS, srcFile); err != nil {
		return files.WrapVfsError(err)
	}
	return nil
}

func deleteSourceFile(srcVFS vfs.VFS, srcFile *vfs.FileDoc) error {
	if err := srcVFS.GetIndexer().DeleteFileDoc(srcFile); err != nil {
		return files.WrapVfsError(err)
	}
	return nil
}

// ensureUniqueName ensures a child name under parentDirID is unique by checking the
// indexer and applying vfs.ConflictName when needed. isFile controls the conflict
// naming strategy on files vs directories.
func ensureUniqueName(v vfs.VFS, parentDirID, name string, isFile bool) (string, error) {
	exists, err := v.GetIndexer().DirChildExists(parentDirID, name)
	if err != nil {
		return "", err
	}
	if !exists {
		return name, nil
	}
	return vfs.ConflictName(v, parentDirID, name, isFile), nil
}

// createLocalDir wraps VFS CreateDir to keep calling sites concise and consistent.
func createLocalDir(v vfs.VFS, newDir *vfs.DirDoc) error {
	if err := v.CreateDir(newDir); err != nil {
		return err
	}
	return nil
}

// ensureRemoteChildDir creates (or reuses) a child dir under the given parent remote dir.
// It tries Mkdir first, falling back to GetDirByPath if it already exists.
func ensureRemoteChildDir(c *client.Client, parent *client.Dir, name string) (string, error) {
	targetPath := parent.Attrs.Fullpath + "/" + name
	newDir, err := c.Mkdir(targetPath)
	if err == nil {
		return newDir.ID, nil
	}
	// If the directory already exists, compute a unique name and create it
	uniqueName, e2 := ensureRemoteUniqueChildName(c, parent.Attrs.Fullpath, name, false)
	if e2 != nil {
		return "", err
	}
	uniquePath := parent.Attrs.Fullpath + "/" + uniqueName
	newDir, e3 := c.Mkdir(uniquePath)
	if e3 == nil {
		return newDir.ID, nil
	}
	// As a last resort, try to fetch it by path (in case of a race)
	existing, e4 := c.GetDirByPath(uniquePath)
	if e4 != nil {
		return "", err
	}
	return existing.ID, nil
}

// ensureRemoteUniqueChildName returns a name that does not exist under the given
// parent fullpath on the remote instance. For files, the numeric suffix is added
// before the file extension (e.g., "name (2).txt"). For directories, it is
// appended at the end (e.g., "name (2)").
func ensureRemoteUniqueChildName(c *client.Client, parentFullpath, name string, isFile bool) (string, error) {
	candidate := name
	// quick existence check
	if _, err := c.GetDirOrFileByPath(parentFullpath + "/" + candidate); err != nil {
		// Not found: use original name
		return candidate, nil
	}

	// Build base name and extension for files
	base := name
	ext := ""
	if isFile {
		if idx := strings.LastIndex(name, "."); idx > 0 {
			base = name[:idx]
			ext = name[idx:]
		}
	}

	// Try with incrementing suffixes starting at 2
	for i := 2; i < 10000; i++ { // large safety cap
		if isFile {
			candidate = base + " (" + strconv.Itoa(i) + ")" + ext
		} else {
			candidate = base + " (" + strconv.Itoa(i) + ")"
		}
		if _, err := c.GetDirOrFileByPath(parentFullpath + "/" + candidate); err != nil {
			return candidate, nil
		}
	}
	return "", files.WrapVfsError(errors.New("could not find unique name on remote"))
}

// uploadWithConflictRetry attempts an upload with the provided name first; if the
// server responds with a 409/Conflict, it retries with an auto-generated unique
// name under parentFullpath. Returns the created file metadata.
func uploadWithConflictRetry(c *client.Client, parentFullpath, dirID, name string, md5sum []byte, contents io.Reader, contentType string, contentLength int64) (*client.File, error) {
	// First, optimistic attempt with original name
	f, err := c.Upload(&client.Upload{
		Name:          name,
		DirID:         dirID,
		ContentMD5:    md5sum,
		Contents:      contents,
		ContentType:   contentType,
		ContentLength: contentLength,
	})
	if err == nil {
		return f, nil
	}
	// If it's not an HTTP 409/Conflict error, just return it
	if reqErr, ok := err.(*request.Error); !ok || reqErr.Title != http.StatusText(http.StatusConflict) {
		return nil, err
	}
	// Compute a unique name and retry once
	uniqueName, uerr := ensureRemoteUniqueChildName(c, parentFullpath, name, true)
	if uerr != nil {
		return nil, uerr
	}
	return c.Upload(&client.Upload{
		Name:          uniqueName,
		DirID:         dirID,
		ContentMD5:    md5sum,
		Contents:      contents,
		ContentType:   contentType,
		ContentLength: contentLength,
	})
}

// planLocalTree builds a flat list of directories and files to move starting at srcRoot.
// The first directory entry is the root. Parent mapping is derived from DirID on docs.
func planLocalTree(v vfs.VFS, srcRoot *vfs.DirDoc, _ string) ([]*vfs.DirDoc, []*vfs.FileDoc, error) {
	var dirs []*vfs.DirDoc
	var filesToMove []*vfs.FileDoc
	dirs = append(dirs, srcRoot)
	if err := vfs.WalkAlreadyLocked(v, srcRoot, func(_ string, d *vfs.DirDoc, f *vfs.FileDoc, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if f != nil {
			filesToMove = append(filesToMove, f)
			return nil
		}
		if d != nil && d.DocID != srcRoot.DocID {
			dirs = append(dirs, d)
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}
	return dirs, filesToMove, nil
}

func extractBearerToken(s *sharing.Sharing) (string, error) {
	if len(s.Credentials) == 0 {
		return "", jsonapi.Forbidden(errors.New("no credentials"))
	}
	return s.Credentials[0].DriveToken, nil
}
