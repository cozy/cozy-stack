package sharings

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/client"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/permission"
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
	copyBufferSize     = 32 * 1024
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
}

// validateMoveRequest checks the structural validity of a move request and
// returns a 400-style error via jsonapi when invalid. It does not perform
// authorization checks.
func validateMoveRequest(req moveRequest) error {
	if req.Source.FileID == "" {
		return jsonapi.BadRequest(errors.New("missing source file_id"))
	}
	if req.Dest.DirID == "" {
		return jsonapi.BadRequest(errors.New("missing dest dir_id"))
	}
	if req.Source.DirID != "" {
		return jsonapi.BadRequest(errors.New("moving directories is not supported, please move the directory content instead"))
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

func moveFileToSharedDrive(c echo.Context, inst *instance.Instance, sourceInstanceURL string,
	targetDirID string, sourceFileID string, s *sharing.Sharing) error {
	localSrcFile, err := inst.VFS().FileByID(sourceFileID)
	if err != nil {
		return files.WrapVfsError(err)
	}

	destURL, err := url.Parse(s.Members[0].Instance)
	if err != nil {
		return files.WrapVfsError(err)
	}
	bearer := s.Credentials[0].DriveToken
	dstClient := NewRemoteClient(destURL, bearer)

	srcHandle, err := inst.VFS().OpenFile(localSrcFile)
	if err != nil {
		return files.WrapVfsError(err)
	}
	defer srcHandle.Close()

	uploaded, err := dstClient.Upload(&client.Upload{
		Name:          localSrcFile.DocName,
		DirID:         targetDirID,
		ContentMD5:    localSrcFile.MD5Sum,
		Contents:      srcHandle,
		ContentType:   localSrcFile.Mime,
		ContentLength: localSrcFile.ByteSize,
	})
	if err != nil {
		return files.WrapVfsError(err)
	}

	if err := deleteSourceFile(inst.VFS(), localSrcFile); err != nil {
		return err
	}

	return respondRemoteUpload(c, uploaded)
}

func moveFileSameStack(c echo.Context, srcInst *instance.Instance, destInst *instance.Instance, fileID string, dirID string) error {
	srcFile, err := srcInst.VFS().FileByID(fileID)
	if err != nil {
		return files.WrapVfsError(err)
	}
	newFileDoc, err := createFileDocFromSource(srcFile, dirID)
	if err != nil {
		return files.WrapVfsError(err)
	}
	if err := copyFileContent(destInst.VFS(), newFileDoc, srcInst.VFS(), srcFile); err != nil {
		return err
	}
	if err := deleteSourceFile(srcInst.VFS(), srcFile); err != nil {
		return err
	}
	obj := files.NewFile(newFileDoc, destInst, nil)
	return jsonapi.Data(c, http.StatusCreated, obj, nil)
}

func moveFileFromSharedDrive(c echo.Context, inst *instance.Instance, sourceInstanceURL string, fileID string,
	dirID string, s *sharing.Sharing) error {
	destDir, err := inst.VFS().DirByID(dirID)
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

	srcFile, err := srcClient.GetFileByID(fileID)
	if err != nil {
		return files.WrapVfsError(err)
	}

	newFileDoc, err := createFileDocFromRemoteFile(srcFile, destDir.DocID)
	if err != nil {
		return files.WrapVfsError(err)
	}

	fd, err := inst.VFS().CreateFile(newFileDoc, nil)
	if err != nil {
		return files.WrapVfsError(err)
	}

	rc, err := srcClient.DownloadByID(fileID)
	if err != nil {
		return files.WrapVfsError(err)
	}
	defer rc.Close()

	// Use a small buffer to reduce allocations during large copies
	if _, err := io.CopyBuffer(fd, rc, make([]byte, copyBufferSize)); err != nil {
		// Best-effort close to avoid leaking descriptors on error paths
		_ = fd.Close()
		return files.WrapVfsError(err)
	}
	err = fd.Close()
	if err != nil {
		return err
	}
	if err := srcClient.PermanentDeleteByID(fileID); err != nil {
		inst.Logger().WithNamespace("move").Warnf("Could not delete source file: %v", err)
	}
	obj := files.NewFile(newFileDoc, inst, nil)
	return jsonapi.Data(c, http.StatusCreated, obj, nil)
}

// MoveHandler is a unified endpoint to move a file between different locations.
// Accepted inputs (JSON body preferred, query params supported for compatibility):
// - Common: file-id, dir-id
// - Source: source-instance + source-sharing_id (both together for shared drives)
// - Destination: dest-instance + dest-sharing_id (both together for shared drives)
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
//		  }
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

	// Authorization: if at least one side is not a shared drive, require whole-type permission
	if req.Source.SharingID == "" || req.Dest.SharingID == "" {
		if err := middlewares.AllowWholeType(c, permission.PATCH, consts.Files); err != nil {
			return err
		}
	}

	sourceInstance, err := resolveInstanceOrCurrent(req.Source.Instance, inst)
	if err != nil {
		return err
	}
	destInstance, err := resolveInstanceOrCurrent(req.Dest.Instance, inst)
	if err != nil {
		return err
	}

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
			return moveFileSameStack(c, sourceInstance, destInstance, req.Source.FileID, req.Dest.DirID)
		} else {
			return moveBetweenSharedDrives(c, req.Source.Instance, req.Source.FileID, sourceSharing, req.Dest.Instance, req.Dest.DirID, destSharing)
		}
	} else if req.Source.Instance != "" && req.Dest.Instance == "" {
		s, err := checkSharedDrivePermission(inst, req.Source.SharingID)
		if err != nil {
			return err
		}
		if OnSameStackCheck(sourceInstance, destInstance) {
			return moveFileSameStack(c, sourceInstance, destInstance, req.Source.FileID, req.Dest.DirID)
		} else {
			return moveFileFromSharedDrive(c, destInstance, req.Source.Instance, req.Source.FileID, req.Dest.DirID, s)
		}
	} else if req.Source.Instance == "" && req.Dest.Instance != "" {
		s, err := checkSharedDrivePermission(inst, req.Dest.SharingID)
		if err != nil {
			return err
		}
		if OnSameStackCheck(sourceInstance, destInstance) {
			return moveFileSameStack(c, sourceInstance, destInstance, req.Source.FileID, req.Dest.DirID)
		} else {
			return moveFileToSharedDrive(c, sourceInstance, req.Dest.Instance, req.Dest.DirID, req.Source.FileID, s)
		}
	} else {
		return jsonapi.BadRequest(errors.New("to move files inside personal drive use patch function"))
	}
}

func moveBetweenSharedDrives(c echo.Context, sourceInstanceURL, fileID string, sourceSharing *sharing.Sharing, destInstanceURL, dirID string, destSharing *sharing.Sharing) error {
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

	uploaded, err := destClient.Upload(&client.Upload{
		Name:          srcFile.Attrs.Name,
		DirID:         dirID,
		ContentMD5:    srcFile.Attrs.MD5Sum,
		Contents:      srcReader,
		ContentType:   srcFile.Attrs.Mime,
		ContentLength: srcFile.Attrs.Size,
	})
	if err != nil {
		return files.WrapVfsError(err)
	}

	if err := sourceClient.PermanentDeleteByID(fileID); err != nil {
		inst.Logger().WithNamespace("move").Warnf("Could not delete source file: %v", err)
	}

	return respondRemoteUpload(c, uploaded)
}

func getInstanceIdentifierFromURL(urlStr string) (*instance.Instance, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	host := u.Host
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}
	inst, err := lifecycle.GetInstance(host)
	if err != nil {
		return nil, err
	}
	return inst, nil
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

// OnSameStackCheck allows tests to override same-stack detection logic.
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

// respondRemoteUpload returns a minimal JSONAPI-like response for a file created
// via remote upload (cross-stack path), matching existing attributes used by clients.
func respondRemoteUpload(c echo.Context, uploaded *client.File) error {
	c.Response().Header().Set("Content-Type", jsonAPIContentType)
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"data": map[string]interface{}{
			"type": "io.cozy.files",
			"id":   uploaded.ID,
			"attributes": map[string]interface{}{
				"name":       uploaded.Attrs.Name,
				"dir_id":     uploaded.Attrs.DirID,
				"type":       uploaded.Attrs.Type,
				"size":       uploaded.Attrs.Size,
				"mime":       uploaded.Attrs.Mime,
				"class":      uploaded.Attrs.Class,
				"executable": uploaded.Attrs.Executable,
				"tags":       uploaded.Attrs.Tags,
			},
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

func extractBearerToken(s *sharing.Sharing) (string, error) {
	if len(s.Credentials) == 0 {
		return "", jsonapi.Forbidden(errors.New("no credentials"))
	}
	return s.Credentials[0].DriveToken, nil
}
