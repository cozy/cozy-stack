package sharings

import (
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
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

type docPatch struct {
	docID string

	vfs.DocPatch
}

// ListSharedDrives returns the list of the shared drives.
func ListSharedDrives(c echo.Context) error {
	if err := middlewares.AllowWholeType(c, permission.GET, consts.Files); err != nil {
		return wrapErrors(err)
	}

	inst := middlewares.GetInstance(c)
	drives, err := sharing.ListDrives(inst)
	if err != nil {
		return wrapErrors(err)
	}

	objs := make([]jsonapi.Object, 0, len(drives))
	for _, drive := range drives {
		obj := &sharing.APISharing{
			Sharing:     drive,
			Credentials: nil,
		}
		objs = append(objs, obj)
	}
	return jsonapi.DataList(c, http.StatusOK, objs, nil)
}

// Load either a DirDoc or a FileDoc from the given `file-id` param. The function also checks permissions
func loadDirOrFileFromParam(c echo.Context, inst *instance.Instance, perm permission.Verb) (*vfs.DirDoc, *vfs.FileDoc, error) {
	dir, file, err := inst.VFS().DirOrFileByID(c.Param("file-id"))
	if err != nil {
		return nil, nil, files.WrapVfsError(err)
	}

	if dir != nil {
		err = middlewares.AllowVFS(c, perm, dir)
	} else {
		err = middlewares.AllowVFS(c, perm, file)
	}
	if err != nil {
		return nil, nil, files.WrapVfsError(err)
	}

	if dir != nil {
		return dir, nil, nil
	}
	return nil, file, nil
}

// Same as `loadDirOrFile` but intolerant of files, responds 422s
func loadDirFromParam(c echo.Context, inst *instance.Instance) (*vfs.DirDoc, error) {
	dir, file, err := loadDirOrFileFromParam(c, inst, permission.GET)
	if file != nil {
		return nil, jsonapi.InvalidParameter("file-id", errors.New("file-id: not a directory"))
	}
	return dir, err
}

// Same as `loadDirOrFile` but intolerant of directories, responds 422s
func loadFileFromParam(c echo.Context, inst *instance.Instance, perm permission.Verb) (*vfs.FileDoc, error) {
	dir, file, err := loadDirOrFileFromParam(c, inst, perm)
	if dir != nil {
		return nil, jsonapi.InvalidParameter("file-id", errors.New("file-id: not a file"))
	}
	return file, err
}

func HeadDirOrFile(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	_, _, err := loadDirOrFileFromParam(c, inst, permission.GET)
	if err != nil {
		return err
	}
	return nil
}

// TODO: reuse files.ReadMetadataFromIDHandler?!
func GetDirOrFileData(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	dir, file, err := loadDirOrFileFromParam(c, inst, permission.GET)
	if err != nil {
		return err
	}
	if dir != nil {
		return files.DirData(c, http.StatusOK, dir, s)
	}
	return files.FileData(c, http.StatusOK, file, true, nil, s)
}

func ReadFileContentFromIDHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	//TODO: CSP ?
	file, err := loadFileFromParam(c, inst, permission.GET)
	if err != nil {
		return err
	}
	//TODO: 403 if check perms is true
	return files.SendFileFromDoc(inst, c, file, false)
}

func ReadFileContentFromVersion(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	file, err := loadFileFromParam(c, inst, permission.GET)
	if err != nil {
		return err
	}

	version, err := vfs.FindVersion(inst, file.DocID+"/"+c.Param("version-id"))
	if err != nil {
		return files.WrapVfsError(err)
	}

	disposition := "inline"
	if c.QueryParam("Dl") == "1" {
		disposition = "attachment"
	}
	err = vfs.ServeFileContent(inst.VFS(), file, version, "", disposition, c.Request(), c.Response())
	if err != nil {
		return files.WrapVfsError(err)
	}

	return nil
}

func GetDirSize(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	fs := inst.VFS()
	dir, err := loadDirFromParam(c, inst)
	if err != nil {
		return err
	}

	size, err := fs.DirSize(dir)
	if err != nil {
		return files.WrapVfsError(err)
	}

	result := files.ApiDiskSize{DocID: dir.DocID, Size: size}
	return jsonapi.Data(c, http.StatusOK, &result, nil)
}

// ModifyMetadataByIDHandler handles PATCH requests on /files/:file-id
//
// It can be used to modify the file or directory metadata, as well as
// moving and renaming it in the filesystem.
func ModifyMetadataByIDHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	patch, err := getPatch(c, c.Param("file-id"))
	if err != nil {
		return files.WrapVfsError(err)
	}
	if err = applyPatch(c, inst.VFS(), patch); err != nil {
		return files.WrapVfsError(err)
	}
	return nil
}

func getPatch(c echo.Context, docID string) (*docPatch, error) {
	var patch docPatch
	obj, err := jsonapi.Bind(c.Request().Body, &patch)
	if err != nil {
		return nil, jsonapi.BadJSON()
	}
	patch.docID = docID
	patch.RestorePath = nil
	if rel, ok := obj.GetRelationship("parent"); ok {
		rid, ok := rel.ResourceIdentifier()
		if !ok {
			return nil, jsonapi.BadJSON()
		}
		patch.DirID = &rid.ID
	}
	return &patch, nil
}

func applyPatch(c echo.Context, fs vfs.VFS, patch *docPatch) (err error) {
	dir, file, err := fs.DirOrFileByID(patch.docID)
	if err != nil {
		return err
	}

	var rev string
	if dir != nil {
		rev = dir.Rev()
	} else {
		rev = file.Rev()
	}

	if err = files.CheckIfMatch(c, rev); err != nil {
		return err
	}

	if dir != nil {
		if err = middlewares.AllowVFS(c, permission.PATCH, dir); err != nil {
			return err
		}
	} else {
		if err = middlewares.AllowVFS(c, permission.PATCH, file); err != nil {
			return err
		}
	}

	if patch.DirID != nil {
		newParent, _, err := fs.DirOrFileByID(*patch.DirID)
		if err != nil {
			return err
		}
		if newParent == nil {
			return jsonapi.BadRequest(errors.New("destination directory does not exist"))
		}
		// XXX: This permission check ensures the new parent is in the shared drive.
		if err = middlewares.AllowVFS(c, permission.POST, newParent); err != nil {
			return err
		}
	}

	if dir != nil {
		files.UpdateDirCozyMetadata(c, dir)
		dir, err = vfs.ModifyDirMetadata(fs, dir, &patch.DocPatch)
	} else {
		files.UpdateFileCozyMetadata(c, file, false)
		file, err = vfs.ModifyFileMetadata(fs, file, &patch.DocPatch)
	}
	if err != nil {
		return err
	}

	if dir != nil {
		return files.DirData(c, http.StatusOK, dir, nil)
	}
	return files.FileData(c, http.StatusOK, file, false, nil, nil)
}

func ChangesFeed(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	// TODO: if owner then fail, shouldn't be accessing their own stuff, risk recursion download kinda thing
	// TODO: should this break if there ever is actually more than 1 directory ?
	// TODO: consider nested sharings
	sharedDir, err := getSharingDir(c, inst, s)
	if err != nil {
		return err
	}
	return files.ChangesFeed(c, inst, sharedDir)
}

func CopyFile(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.CopyFile(c, inst, s)
}

// createFileDocFromSource creates a new file document based on a source file
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
		false, // trashed
		false, // encrypted
		srcFile.Tags,
	)
}

// copyFileContent copies file content between instances
func copyFileContent(destVFS vfs.VFS, newFileDoc *vfs.FileDoc, srcVFS vfs.VFS, srcFile *vfs.FileDoc) error {
	if err := destVFS.CopyFileFromOtherFS(newFileDoc, nil, srcVFS, srcFile); err != nil {
		return files.WrapVfsError(err)
	}
	return nil
}

// deleteSourceFile deletes a file from the source instance
func deleteSourceFile(srcVFS vfs.VFS, srcFile *vfs.FileDoc) error {
	if err := srcVFS.GetIndexer().DeleteFileDoc(srcFile); err != nil {
		return files.WrapVfsError(err)
	}
	return nil
}

func MoveDownstreamSameStack(c echo.Context, inst *instance.Instance, sourceInstance *instance.Instance, targetDirId string, sourceFileID string) error {
	// Check if the source file exists and get its metadata
	srcFile, err := sourceInstance.VFS().FileByID(sourceFileID)
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Create a new file document based on the source file
	newFileDoc, err := createFileDocFromSource(srcFile, targetDirId)
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Copy the file content from the source instance
	if err := copyFileContent(inst.VFS(), newFileDoc, sourceInstance.VFS(), srcFile); err != nil {
		return err
	}

	// Delete the file from the source instance
	if err := deleteSourceFile(sourceInstance.VFS(), srcFile); err != nil {
		return err
	}

	// Return the new file document
	obj := files.NewFile(newFileDoc, inst, nil)
	return jsonapi.Data(c, http.StatusCreated, obj, nil)
}

// extractBearerToken extracts the bearer token from the Authorization header
func extractBearerToken(c echo.Context) string {
	var bearer string
	if auth := c.Request().Header.Get("Authorization"); auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			bearer = parts[1]
		}
	}
	return bearer
}

// createFileDocFromRemoteFile creates a new file document based on a remote file
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
		false, // trashed
		remoteFile.Attrs.Encrypted,
		remoteFile.Attrs.Tags,
	)
}

func MoveDownstreamDifferentStack(c echo.Context, inst *instance.Instance, sourceInstanceURL string, targetDirId string, sourceFileID string) error {
	// Build a client for the source instance
	u, err := url.Parse(sourceInstanceURL)
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Build the remote client (overridable in tests)
	bearer := extractBearerToken(c)
	srcClient := NewRemoteClient(u, bearer)

	// Get source file metadata
	srcFile, err := srcClient.GetFileByID(sourceFileID)
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Create destination file doc using source metadata
	newFileDoc, err := createFileDocFromRemoteFile(srcFile, targetDirId)
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Create the file in the destination VFS
	fd, err := inst.VFS().CreateFile(newFileDoc, nil)
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Download content from source and write into destination
	rc, err := srcClient.DownloadByID(sourceFileID)
	if err != nil {
		return files.WrapVfsError(err)
	}
	defer rc.Close()

	if _, err := io.Copy(fd, rc); err != nil {
		return files.WrapVfsError(err)
	}

	err = fd.Close()
	if err != nil {
		return err
	}

	// Best effort: delete the source file
	if err := srcClient.TrashByID(sourceFileID); err != nil {
		log.Printf("Warning: Could not delete source file: %v", err)
	}

	obj := files.NewFile(newFileDoc, inst, nil)
	return jsonapi.Data(c, http.StatusCreated, obj, nil)
}

// validateInstanceURL validates an instance URL and returns an error if it's invalid
func validateInstanceURL(instanceURL string, paramName string) error {
	if instanceURL == "" {
		return jsonapi.BadRequest(errors.New("missing " + paramName + " parameter"))
	}
	if _, err := url.Parse(instanceURL); err != nil {
		return jsonapi.BadRequest(errors.New("invalid " + paramName + " parameter"))
	}
	return nil
}

// validateFileID validates a file ID and returns an error if it's invalid
func validateFileID(fileID string) error {
	if fileID == "" {
		return jsonapi.BadRequest(errors.New("missing file-id parameter"))
	}
	return nil
}

// validateDirID validates a directory ID, checks if it exists, and returns an error if it's invalid
func validateDirID(dirID string, inst *instance.Instance) error {
	if dirID == "" {
		return jsonapi.BadRequest(errors.New("missing dir-id parameter"))
	}
	// Check if the directory exists
	_, err := inst.VFS().DirByID(dirID)
	if err != nil {
		return files.WrapVfsError(err)
	}
	return nil
}

// TODO we can move not only a file but a directory
// moveDownstream handles the common logic for moving a file downstream
func moveDownstream(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	// instance url, from where we are going to move the file
	sourceInstanceURL := c.QueryParam("source-instance")
	if err := validateInstanceURL(sourceInstanceURL, "source-instance"); err != nil {
		return err
	}

	sourceInstance, err := getInstanceIdentifierFromURL(sourceInstanceURL)
	if err != nil {
		// if we can't get source instance it will be cross stack request
		sourceInstance = nil
	}

	// file identifier to move
	fileID := c.QueryParam("file-id")
	if err := validateFileID(fileID); err != nil {
		return err
	}

	// Identifier of the directory in the current instance to move the file
	dirID := c.Param("dir-id")
	if err := validateDirID(dirID, inst); err != nil {
		return err
	}

	if sourceInstance != nil && OnSameStackCheck(sourceInstance, inst) {
		return MoveDownstreamSameStack(c, inst, sourceInstance, dirID, fileID)
	} else {
		return MoveDownstreamDifferentStack(c, inst, sourceInstanceURL, dirID, fileID)
	}
}

// MoveDownstreamHandler handles moving a file from a source instance to the current instance
func MoveDownstreamHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return moveDownstream(c, inst, s)
}

// moveUpstreamSameStack handles moving a file upstream when both instances are on the same stack
func moveUpstreamSameStack(c echo.Context, srcInst *instance.Instance, destInst *instance.Instance, srcFile *vfs.FileDoc, dirID string) error {
	// Create a new file document based on the source file
	newFileDoc, err := createFileDocFromSource(srcFile, dirID)
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Copy the file content to the destination instance
	if err := copyFileContent(destInst.VFS(), newFileDoc, srcInst.VFS(), srcFile); err != nil {
		return err
	}

	// Delete the file from the source instance
	if err := deleteSourceFile(srcInst.VFS(), srcFile); err != nil {
		return err
	}

	// Return the new file document
	obj := files.NewFile(newFileDoc, destInst, nil)
	return jsonapi.Data(c, http.StatusCreated, obj, nil)
}

// moveUpstreamDifferentStack handles moving a file upstream when instances are on different stacks
func moveUpstreamDifferentStack(c echo.Context, srcInst *instance.Instance, destInstanceURL string, srcFile *vfs.FileDoc, dirID string) error {
	// Parse the destination URL
	u, err := url.Parse(destInstanceURL)
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Extract bearer token from the request
	bearer := extractBearerToken(c)
	dstClient := NewRemoteClient(u, bearer)

	// Open the source file for reading
	srcFileHandle, err := srcInst.VFS().OpenFile(srcFile)
	if err != nil {
		return files.WrapVfsError(err)
	}
	defer srcFileHandle.Close()

	// Upload the file to the destination
	uploaded, err := dstClient.Upload(&client.Upload{
		Name:          srcFile.DocName,
		DirID:         dirID,
		ContentMD5:    srcFile.MD5Sum,
		Contents:      srcFileHandle,
		ContentType:   srcFile.Mime,
		ContentLength: srcFile.ByteSize,
	})
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Delete the file from the source instance
	if err := deleteSourceFile(srcInst.VFS(), srcFile); err != nil {
		return err
	}

	// Return success response mirroring created remote file
	c.Response().Header().Set("Content-Type", "application/vnd.api+json")
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

// moveUpstream handles the common logic for moving a file upstream
func moveUpstream(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	// Get the destination instance URL from query parameter
	destInstanceURL := c.QueryParam("dest-instance")
	if err := validateInstanceURL(destInstanceURL, "dest-instance"); err != nil {
		return err
	}

	// Get the file ID from path parameter
	fileID := c.Param("file-id")
	if err := validateFileID(fileID); err != nil {
		return err
	}

	// Get the destination directory ID
	dirID := c.QueryParam("dir-id")
	if err := validateDirID(dirID, inst); err != nil {
		return err
	}

	// Check if the source file exists and get its metadata
	srcFile, err := inst.VFS().FileByID(fileID)
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Try to get the destination instance to check if it's on the same stack
	destInstance, err := getInstanceIdentifierFromURL(destInstanceURL)
	if err != nil {
		// If we can't get the instance, treat it as cross-stack
		destInstance = nil
	}

	if destInstance != nil && OnSameStackCheck(inst, destInstance) {
		return moveUpstreamSameStack(c, inst, destInstance, srcFile, dirID)
	} else {
		return moveUpstreamDifferentStack(c, inst, destInstanceURL, srcFile, dirID)
	}
}

// TODO we can move not only a file but a directory
func MoveUpstreamHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return moveUpstream(c, inst, s)
}

func CreationHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.Create(c, s)
}

func DestroyFileHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.DestroyFileHandler(c)
}

func OverwriteFileContentHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.OverwriteFileContent(c, s)
}

func RestoreTrashFileHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.Restore(c, s)
}

func TrashHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.Trash(c, s)
}

func UploadMetadataHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.UploadMetadataHandler(c)
}

func ThumbnailHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.ThumbnailHandler(c)
}

func FileDownloadCreateHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.FileDownload(c, s)
}

func FileDownloadHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	return files.FileDownloadHandler(c)
}

// Find the directory linked to the drive sharing and return it if the user
// requesting it has the proper permissions.
func getSharingDir(c echo.Context, inst *instance.Instance, s *sharing.Sharing) (*vfs.DirDoc, error) {
	fs := inst.VFS()
	rule := s.FirstFilesRule()
	if rule != nil {
		if rule.Mime != "" {
			inst.Logger().WithNamespace("drive-proxy").
				Warnf("getSharingDir called for only one file: %s", s.SID)
			return nil, jsonapi.BadRequest(errors.New("not a shared drive"))
		}
		dir, _ := fs.DirByID(rule.Values[0])
		if dir != nil {
			return dir, nil
		}
	}

	return nil, jsonapi.NotFound(errors.New("shared drive not found"))
}

// drivesRoutes sets the routing for the shared drives
func drivesRoutes(router *echo.Group) {
	group := router.Group("/drives")
	group.GET("", ListSharedDrives)

	drive := group.Group("/:id")

	// TODO: drive.HEAD("/download", proxyPath(ReadFileContentFromPathHandler))
	// TODO: drive.GET("/download", proxyPath(ReadFileContentFromPathHandler))
	drive.HEAD("/download/:file-id", proxy(ReadFileContentFromIDHandler))
	drive.GET("/download/:file-id", proxy(ReadFileContentFromIDHandler))

	drive.HEAD("/download/:file-id/:version-id", proxy(ReadFileContentFromVersion))
	drive.GET("/download/:file-id/:version-id", proxy(ReadFileContentFromVersion))
	// TODO: drive.POST("/revert/:file-id/:version-id", RevertFileVersion)
	// TODO: drive.PATCH("/:file-id/:version-id", ModifyFileVersionMetadata)
	// TODO: drive.DELETE("/:file-id/:version-id", DeleteFileVersionMetadata)
	// TODO: drive.POST("/:file-id/versions", CopyVersionHandler)
	// TODO: drive.DELETE("/versions", ClearOldVersions)

	// TODO: drive.POST("/_all_docs", GetAllDocs)
	// WONT: drive.POST("/_find", FindFilesMango)
	drive.GET("/_changes", proxy(ChangesFeed))

	drive.HEAD("/:file-id", proxy(HeadDirOrFile))

	// TODO: drive.GET("/metadata", ReadMetadataFromPathHandler)
	drive.GET("/:file-id", proxy(GetDirOrFileData))
	// TODO: drive.GET("/:file-id/relationships/contents", GetChildrenHandler)
	drive.GET("/:file-id/size", proxy(GetDirSize))

	// TODO: drive.PATCH("/metadata", ModifyMetadataByPathHandler)
	drive.PATCH("/:file-id", proxy(ModifyMetadataByIDHandler))
	// TODO: drive.PATCH("/", ModifyMetadataByIDInBatchHandler)

	drive.POST("/", proxy(CreationHandler))
	drive.POST("/:file-id", proxy(CreationHandler))
	drive.PUT("/:file-id", proxy(OverwriteFileContentHandler))
	drive.POST("/upload/metadata", proxy(UploadMetadataHandler))
	drive.POST("/:file-id/copy", proxy(CopyFile))
	drive.POST("/:dir-id/downstream", proxy(MoveDownstreamHandler))
	drive.POST("/:file-id/upstream", proxy(MoveUpstreamHandler))

	drive.GET("/:file-id/thumbnails/:secret/:format", proxy(ThumbnailHandler))

	// TODO: drive.POST("/archive", ArchiveDownloadCreateHandler)
	// TODO: drive.GET("/archive/:secret/:fake-name", ArchiveDownloadHandler)

	drive.POST("/downloads", proxy(FileDownloadCreateHandler))
	drive.GET("/downloads/:secret/:fake-name", proxy(FileDownloadHandler))

	// TODO: drive.POST("/:file-id/relationships/referenced_by", AddReferencedHandler)
	// TODO: drive.DELETE("/:file-id/relationships/referenced_by", RemoveReferencedHandler)

	// TODO: drive.GET("/trash", ReadTrashFilesHandler)
	// TODO: drive.DELETE("/trash", ClearTrashHandler)

	drive.POST("/trash/:file-id", proxy(RestoreTrashFileHandler))
	drive.DELETE("/trash/:file-id", proxy(DestroyFileHandler))

	drive.DELETE("/:file-id", proxy(TrashHandler))
}

func proxy(fn func(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error) echo.HandlerFunc {
	return func(c echo.Context) error {
		inst := middlewares.GetInstance(c)
		s, err := sharing.FindSharing(inst, c.Param("id"))
		if err != nil {
			return wrapErrors(err)
		}
		if !s.Drive {
			return jsonapi.NotFound(errors.New("not a drive"))
		}

		if s.Owner {
			return fn(c, inst, s)
		}

		// On a recipient, we proxy the request to the owner
		method := c.Request().Method
		if method == http.MethodHead {
			method = http.MethodGet
		}
		verb := permission.Verb(method)
		if err := middlewares.AllowWholeType(c, verb, consts.Files); err != nil {
			return err
		}
		if len(s.Credentials) == 0 {
			return jsonapi.InternalServerError(errors.New("no credentials"))
		}
		token := s.Credentials[0].DriveToken
		u, err := url.Parse(s.Members[0].Instance)
		if err != nil {
			return jsonapi.InternalServerError(err)
		}

		// XXX Let's try to avoid one http request by cheating a bit. If the two
		// instances are on the same domain (same stack), we can call directly
		// the handler. We just need to fake the middlewares. It helps for
		// performances.
		if owner, err := lifecycle.GetInstance(u.Host); err == nil {
			pdoc, err := permission.GetForShareCode(owner, token)
			if err != nil {
				return err
			}
			middlewares.ForcePermission(c, pdoc)
			middlewares.SetInstance(c, owner)
			return fn(c, owner, s)
		}

		director := func(req *http.Request) {
			req.URL = u
			req.URL.Path = c.Request().URL.Path
			req.URL.RawQuery = c.Request().URL.RawQuery
			req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
			req.Header.Del(echo.HeaderCookie)
			req.Header.Del("Host")
		}
		proxy := &httputil.ReverseProxy{Director: director}
		logger := inst.Logger().WithNamespace("drive-proxy").Writer()
		defer logger.Close()
		proxy.ErrorLog = log.New(logger, "", 0)
		proxy.ServeHTTP(c.Response(), c.Request())
		return nil
	}
}

// getInstanceIdentifierFromURL extracts the cozy instance identifier from a URL
// For example, from "https://user.cozy.cloud" it returns the instance object for "user.cozy.cloud"
func getInstanceIdentifierFromURL(urlStr string) (*instance.Instance, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	// Remove port if present
	host := u.Host
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}

	// Get the instance object using the hostname
	inst, err := lifecycle.GetInstance(host)
	if err != nil {
		return nil, err
	}

	return inst, nil
}

// onSameStack checks if two instances are running on the same stack (same port)
// by comparing their domain ports.
// we consider that if we can find instancy by url then thet are on the same server
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

// OnSameStackCheck allows tests to override same-stack detection.
// Default to onSameStack; tests can replace it.
var OnSameStackCheck = onSameStack

// NewRemoteClient allows tests to override how the remote client is constructed
// for cross-stack file operations. By default, it builds a client using the
// provided URL and optional bearer token.
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
