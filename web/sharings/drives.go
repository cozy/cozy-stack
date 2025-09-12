package sharings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

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

func MoveDownstreamSameStack(c echo.Context, inst *instance.Instance, sourceInstance *instance.Instance, targetDirId string, sourceFileID string) error {
	// Check if the source file exists and get its metadata
	srcFile, err := sourceInstance.VFS().FileByID(sourceFileID)
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Just move the file using CopyFileFromOtherFS for same-stack optimization
	newFileDoc, err := vfs.NewFileDoc(
		srcFile.DocName,
		targetDirId,
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
	if err != nil {
		return files.WrapVfsError(err)
	}

	//var file, e = inst.VFS().CreateFile(newFileDoc, nil)
	//if e != nil {
	//	return err
	//}
	//file.Close()

	// Copy the file content from the source instance
	if err := inst.VFS().CopyFileFromOtherFS(newFileDoc, nil, sourceInstance.VFS(), srcFile); err != nil {
		return files.WrapVfsError(err)
	}

	// Delete the file from the source instance
	if err := sourceInstance.VFS().GetIndexer().DeleteFileDoc(srcFile); err != nil {
		return files.WrapVfsError(err)
	}

	// Return the new file document
	obj := files.NewFile(newFileDoc, inst, nil)
	return jsonapi.Data(c, http.StatusCreated, obj, nil)
}

func MoveDownstreamDifferentStack(c echo.Context, inst *instance.Instance, sourceInstanceURL string, targetDirId string, sourceFileID string) error {
	//TODO get information about source file with HTTP call

	// Cross-stack move: download and save the file via HTTP
	// Create a new file document in the destination
	//TODO create a new file with data from HTTP call
	var newFileDoc, err = vfs.NewFileDoc(
		"srcFile.DocName",
		targetDirId,
		0,
		nil,
		"srcFile.Mime",
		"srcFile.Class",
		time.Now(),
		false,
		false, // trashed
		false, // encrypted
		nil,
	)
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Create the file in the destination VFS
	_, err = inst.VFS().CreateFile(newFileDoc, nil)
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Download the file content from the source instance via HTTP
	if !strings.HasSuffix(sourceInstanceURL, "/") {
		sourceInstanceURL += "/"
	}

	// Make HTTP request to download the file content
	downloadURL := sourceInstanceURL + "files/" + sourceFileID + "/content"
	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return files.WrapVfsError(err)
	}

	// TODO implement authorization
	// Add authorization header if we have credentials

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return files.WrapVfsError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return files.WrapVfsError(fmt.Errorf("failed to download file from source instance: %s", resp.Status))
	}

	// Open the destination file for writing
	destFileHandle, err := inst.VFS().OpenFile(newFileDoc)
	if err != nil {
		return files.WrapVfsError(err)
	}
	defer destFileHandle.Close()

	// Copy the file content from HTTP response to destination file
	if _, err := io.Copy(destFileHandle, resp.Body); err != nil {
		return files.WrapVfsError(err)
	}

	// Close the destination file to ensure content is written
	destFileHandle.Close()

	// Delete the file from the source instance via HTTP API
	deleteURL := sourceInstanceURL + "files/" + sourceFileID
	deleteReq, err := http.NewRequest("DELETE", deleteURL, nil)
	if err != nil {
		// Log the error but don't fail the operation - the file was copied successfully
		log.Printf("Warning: Could not create delete request for source file: %v", err)
	} else {
		// TODO: Add proper authorization headers here
		deleteResp, err := client.Do(deleteReq)
		if err != nil {
			log.Printf("Warning: Could not delete source file: %v", err)
		} else {
			deleteResp.Body.Close()
			if deleteResp.StatusCode != http.StatusOK && deleteResp.StatusCode != http.StatusNoContent {
				log.Printf("Warning: Failed to delete source file, status: %s", deleteResp.Status)
			}
		}
	}

	obj := files.NewFile(newFileDoc, inst, nil)
	return jsonapi.Data(c, http.StatusCreated, obj, nil)
}

// TODO we can move not only a file but a directory
func MoveDownstreamHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	// instance url, from where we are going to move the file
	sourceInstanceURL := c.QueryParam("source-instance")
	if sourceInstanceURL == "" {
		return jsonapi.BadRequest(errors.New("missing source-instance param"))
	}
	// Validate the destination instance URL
	if _, err := url.Parse(sourceInstanceURL); err != nil {
		return jsonapi.BadRequest(errors.New("invalid source-instance parameter"))
	}

	sourceInstance, err := GetInstanceIdentifierFromURL(sourceInstanceURL)
	if err != nil {
		// if we can't get source instance it will be cross stack request
		sourceInstance = nil
	}

	// file identifier to move
	fileID := c.QueryParam("file-id")
	if fileID == "" {
		return jsonapi.BadRequest(errors.New("missing file-id parameter"))
	}

	// Identifier of the directory in the current instance to move the file
	dirID := c.Param("dir-id")
	if dirID == "" {
		return jsonapi.BadRequest(errors.New("missing dir-id parameter"))
	}

	// Check if the destination directory exists
	_, err = inst.VFS().DirByID(dirID)
	if err != nil {
		return files.WrapVfsError(err)
	}

	if sourceInstance != nil && onSameStack(sourceInstance, inst) {
		return MoveDownstreamSameStack(c, inst, sourceInstance, dirID, fileID)
	} else {
		return MoveDownstreamDifferentStack(c, inst, sourceInstanceURL, dirID, fileID)
	}
}

// TODO we can move not only a file but a directory
func MoveUpstreamHandler(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	// Get the destination instance URL from query parameter
	destInstanceURL := c.QueryParam("dest-instance")
	if destInstanceURL == "" {
		return jsonapi.BadRequest(errors.New("missing dest-instance parameter"))
	}

	// Get the file ID from path parameter
	fileID := c.Param("file-id")
	if fileID == "" {
		return jsonapi.BadRequest(errors.New("missing file-id parameter"))
	}

	// Get the destination directory ID from path parameter (or query fallback)
	dirID := c.Param("dir-id")
	if dirID == "" {
		dirID = c.QueryParam("dir-id")
	}
	if dirID == "" {
		return jsonapi.BadRequest(errors.New("missing dir-id parameter"))
	}

	// Validate the destination instance URL
	if _, err := url.Parse(destInstanceURL); err != nil {
		return jsonapi.BadRequest(errors.New("invalid dest-instance parameter"))
	}

	// Check if the source file exists and get its metadata
	srcFile, err := inst.VFS().FileByID(fileID)
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Check if the destination directory exists
	_, err = inst.VFS().DirByID(dirID)
	if err != nil {
		return files.WrapVfsError(err)
	}

	// Try to get the destination instance to check if it's on the same stack
	destInstance, err := GetInstanceIdentifierFromURL(destInstanceURL)
	if err != nil {
		// If we can't get the instance, treat it as cross-stack
		destInstance = nil
	}

	if destInstance != nil && onSameStack(inst, destInstance) {
		// Same-stack optimization: use VFS operations directly
		newFileDoc, err := vfs.NewFileDoc(
			srcFile.DocName,
			dirID,
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
		if err != nil {
			return files.WrapVfsError(err)
		}

		// Copy the file content to the destination instance
		if err := destInstance.VFS().CopyFileFromOtherFS(newFileDoc, nil, inst.VFS(), srcFile); err != nil {
			return files.WrapVfsError(err)
		}

		// Delete the file from the source instance
		if err := inst.VFS().GetIndexer().DeleteFileDoc(srcFile); err != nil {
			return files.WrapVfsError(err)
		}

		// Return the new file document
		obj := files.NewFile(newFileDoc, destInstance, nil)
		return jsonapi.Data(c, http.StatusCreated, obj, nil)
	} else {
		// Cross-stack move: upload file via HTTP
		// Create a new file document for the destination
		newFileDoc, err := vfs.NewFileDoc(
			srcFile.DocName,
			dirID,
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
		if err != nil {
			return files.WrapVfsError(err)
		}

		// Ensure the destination URL ends with a slash
		if !strings.HasSuffix(destInstanceURL, "/") {
			destInstanceURL += "/"
		}

		// Create the file on the destination instance via HTTP
		createURL := destInstanceURL + "files/"
		createData := map[string]interface{}{
			"type": "io.cozy.files",
			"attributes": map[string]interface{}{
				"name":       newFileDoc.DocName,
				"dir_id":     newFileDoc.DirID,
				"type":       newFileDoc.Type,
				"size":       newFileDoc.ByteSize,
				"mime":       newFileDoc.Mime,
				"class":      newFileDoc.Class,
				"executable": newFileDoc.Executable,
				"tags":       newFileDoc.Tags,
			},
		}

		createJSON, err := json.Marshal(createData)
		if err != nil {
			return files.WrapVfsError(err)
		}

		createReq, err := http.NewRequest("POST", createURL, bytes.NewBuffer(createJSON))
		if err != nil {
			return files.WrapVfsError(err)
		}
		createReq.Header.Set("Content-Type", "application/vnd.api+json")

		// TODO: Add proper authorization headers here
		// This would typically include:
		// - Bearer token for authentication
		// - API key for cross-instance operations
		// - Verification that the source instance has permission to create files

		client := &http.Client{}
		createResp, err := client.Do(createReq)
		if err != nil {
			return files.WrapVfsError(err)
		}
		defer createResp.Body.Close()

		if createResp.StatusCode != http.StatusCreated {
			return files.WrapVfsError(fmt.Errorf("failed to create file on destination instance: %s", createResp.Status))
		}

		// Parse the response to get the created file ID
		var createResponse map[string]interface{}
		if err := json.NewDecoder(createResp.Body).Decode(&createResponse); err != nil {
			return files.WrapVfsError(err)
		}

		// Extract the file ID from the response
		data, ok := createResponse["data"].(map[string]interface{})
		if !ok {
			return files.WrapVfsError(errors.New("invalid response format from destination instance"))
		}

		createdFileID, ok := data["id"].(string)
		if !ok {
			return files.WrapVfsError(errors.New("could not get file ID from destination instance response"))
		}

		// Upload the file content to the destination instance
		uploadURL := destInstanceURL + "files/" + createdFileID + "/content"

		// Open the source file for reading
		srcFileHandle, err := inst.VFS().OpenFile(srcFile)
		if err != nil {
			return files.WrapVfsError(err)
		}
		defer srcFileHandle.Close()

		uploadReq, err := http.NewRequest("PUT", uploadURL, srcFileHandle)
		if err != nil {
			return files.WrapVfsError(err)
		}
		uploadReq.Header.Set("Content-Type", srcFile.Mime)
		uploadReq.Header.Set("Content-Length", fmt.Sprintf("%d", srcFile.ByteSize))

		// TODO: Add proper authorization headers here
		uploadResp, err := client.Do(uploadReq)
		if err != nil {
			return files.WrapVfsError(err)
		}
		defer uploadResp.Body.Close()

		if uploadResp.StatusCode != http.StatusOK && uploadResp.StatusCode != http.StatusNoContent {
			return files.WrapVfsError(fmt.Errorf("failed to upload file content to destination instance: %s", uploadResp.Status))
		}

		// Delete the file from the source instance
		if err := inst.VFS().GetIndexer().DeleteFileDoc(srcFile); err != nil {
			return files.WrapVfsError(err)
		}

		// Return success response
		return c.JSON(http.StatusCreated, map[string]interface{}{
			"data": map[string]interface{}{
				"type": "io.cozy.files",
				"id":   createdFileID,
				"attributes": map[string]interface{}{
					"name":       newFileDoc.DocName,
					"dir_id":     newFileDoc.DirID,
					"type":       newFileDoc.Type,
					"size":       newFileDoc.ByteSize,
					"mime":       newFileDoc.Mime,
					"class":      newFileDoc.Class,
					"executable": newFileDoc.Executable,
					"tags":       newFileDoc.Tags,
				},
			},
		})
	}
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

// GetInstanceIdentifierFromURL extracts the cozy instance identifier from a URL and returns the instance object.
// It returns the instance object for the given URL, which typically represents the instance identifier.
// For example, from "https://user.cozy.cloud" it returns the instance object for "user.cozy.cloud"
func GetInstanceIdentifierFromURL(urlStr string) (*instance.Instance, error) {
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
