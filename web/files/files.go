// Package files is the HTTP frontend of the vfs package. It exposes
// an HTTP api to manipulate the filesystem and offer all the
// possibilities given by the vfs.
package files

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/vfs"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
)

// TagSeparator is the character separating tags
const TagSeparator = ","

const (
	fileType   = "io.cozy.files"
	folderType = "io.cozy.folders"
)

// ErrDocTypeInvalid is used when the document type sent is not
// recognized
var ErrDocTypeInvalid = errors.New("Invalid document type")

// CreationHandler handle all POST requests on /files/:folder-id
// aiming at creating a new document in the FS. Given the Type
// parameter of the request, it will either upload a new file or
// create a new directory.
//
// swagger:route POST /files/:folder-id files uploadFileOrCreateDir
func CreationHandler(c *gin.Context) {
	instance := middlewares.GetInstance(c)
	var doc jsonapi.Object
	var err error
	switch c.Query("Type") {
	case fileType:
		doc, err = createFileHandler(c, instance)
	case folderType:
		doc, err = createDirectoryHandler(c, instance)
	default:
		err = ErrDocTypeInvalid
	}

	if err != nil {
		jsonapi.AbortWithError(c, wrapVfsError(err))
		return
	}

	jsonapi.Data(c, http.StatusCreated, doc, nil)
}

func createFileHandler(c *gin.Context, vfsC vfs.Context) (doc *vfs.FileDoc, err error) {
	doc, err = fileDocFromReq(
		c,
		c.Query("Name"),
		c.Param("folder-id"),
		strings.Split(c.Query("Tags"), TagSeparator),
	)
	if err != nil {
		return
	}

	file, err := vfs.CreateFile(vfsC, doc, nil)
	if err != nil {
		return
	}

	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	_, err = io.Copy(file, c.Request.Body)
	return
}

func createDirectoryHandler(c *gin.Context, vfsC vfs.Context) (*vfs.DirDoc, error) {
	tags := strings.Split(c.Query("Tags"), TagSeparator)
	path := c.Query("Path")

	if path != "" {
		if c.Query("Recursive") == "true" {
			return vfs.MkdirAll(vfsC, path, tags)
		}
		return vfs.Mkdir(vfsC, path, tags)
	}

	name, folderID := c.Query("Name"), c.Param("folder-id")
	doc, err := vfs.NewDirDoc(name, folderID, tags, nil)
	if err != nil {
		return nil, err
	}

	if err = vfs.CreateDir(vfsC, doc); err != nil {
		return nil, err
	}

	return doc, nil
}

// OverwriteFileContentHandler handles PUT requests on /files/:file-id
// to overwrite the content of a file given its identifier.
//
// swagger:route PUT /files/:file-id files overwriteFileContent
func OverwriteFileContentHandler(c *gin.Context) {
	var err error
	var instance = middlewares.GetInstance(c)
	var olddoc *vfs.FileDoc
	var newdoc *vfs.FileDoc

	olddoc, err = vfs.GetFileDoc(instance, c.Param("file-id"))
	if err != nil {
		jsonapi.AbortWithError(c, wrapVfsError(err))
		return
	}

	newdoc, err = fileDocFromReq(
		c,
		olddoc.Name,
		olddoc.FolderID,
		olddoc.Tags,
	)
	if err != nil {
		jsonapi.AbortWithError(c, wrapVfsError(err))
		return
	}

	if err = checkIfMatch(c, olddoc.Rev()); err != nil {
		jsonapi.AbortWithError(c, wrapVfsError(err))
		return
	}

	file, err := vfs.CreateFile(instance, newdoc, olddoc)
	if err != nil {
		jsonapi.AbortWithError(c, wrapVfsError(err))
		return
	}

	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
		if err != nil {
			jsonapi.AbortWithError(c, wrapVfsError(err))
		} else {
			jsonapi.Data(c, http.StatusOK, newdoc, nil)
		}
	}()

	_, err = io.Copy(file, c.Request.Body)
	return
}

// ModificationHandler handles PATCH requests on /files/:file-id and
// /files/metadata.
//
// It can be used to modify the file or directory metadata, as well as
// moving and renaming it in the filesystem.
func ModificationHandler(c *gin.Context) {
	var err error

	instance := middlewares.GetInstance(c)

	patch := &vfs.DocPatch{}

	var obj *jsonapi.ObjectMarshalling
	if obj, err = jsonapi.Bind(c.Request, &patch); err != nil {
		jsonapi.AbortWithError(c, jsonapi.BadJSON())
		return
	}

	if rel, ok := obj.GetRelationship("parent"); ok {
		rid, ok := rel.ResourceIdentifier()
		if !ok {
			jsonapi.AbortWithError(c, jsonapi.BadJSON())
			return
		}
		patch.FolderID = &rid.ID
	}

	fileID := c.Param("file-id")

	var file *vfs.FileDoc
	var dir *vfs.DirDoc

	if fileID == "metadata" {
		dir, file, err = vfs.GetDirOrFileDocFromPath(instance, c.Query("Path"), false)
	} else {
		dir, file, err = vfs.GetDirOrFileDoc(instance, fileID, false)
	}

	if err != nil {
		jsonapi.AbortWithError(c, wrapVfsError(err))
		return
	}

	var doc couchdb.Doc
	if dir != nil {
		doc = dir
	} else {
		doc = file
	}

	if err = checkIfMatch(c, doc.Rev()); err != nil {
		jsonapi.AbortWithError(c, wrapVfsError(err))
		return
	}

	var data jsonapi.Object
	if fileDoc, ok := doc.(*vfs.FileDoc); ok {
		data, err = vfs.ModifyFileMetadata(instance, fileDoc, patch)
	} else if dirDoc, ok := doc.(*vfs.DirDoc); ok {
		data, err = vfs.ModifyDirMetadata(instance, dirDoc, patch)
	}

	if err != nil {
		jsonapi.AbortWithError(c, wrapVfsError(err))
		return
	}

	jsonapi.Data(c, http.StatusOK, data, nil)
}

// ReadMetadataFromIDHandler handles all GET requests on /files/:file-
// id aiming at getting file metadata from its path.
//
// swagger:route GET /files/:file-id files getFileMetadata
func ReadMetadataFromIDHandler(c *gin.Context, fileID string) {
	var err error

	instance := middlewares.GetInstance(c)

	dir, file, err := vfs.GetDirOrFileDoc(instance, fileID, true)
	if err != nil {
		jsonapi.AbortWithError(c, wrapVfsError(err))
		return
	}

	var data jsonapi.Object
	if dir != nil {
		data = dir
	} else {
		data = file
	}

	jsonapi.Data(c, http.StatusOK, data, nil)
}

// ReadMetadataFromPathHandler handles all GET requests on
// /files/metadata aiming at getting file metadata from its path.
//
// swagger:route GET /files/metadata files getFileMetadata
func ReadMetadataFromPathHandler(c *gin.Context) {
	var err error

	instance := middlewares.GetInstance(c)

	dir, file, err := vfs.GetDirOrFileDocFromPath(instance, c.Query("Path"), true)
	if err != nil {
		jsonapi.AbortWithError(c, wrapVfsError(err))
		return
	}

	var data jsonapi.Object
	if dir != nil {
		data = dir
	} else {
		data = file
	}

	jsonapi.Data(c, http.StatusOK, data, nil)
}

// ReadFileContentHandler handles all GET requests on /files/:file-id
// aiming at downloading a file. It serves two main purposes in this
// regard:
//  - downloading a file given its ID in inline mode
//  - downloading a file given its path in attachment mode on the
//    /files/download endpoint
//
// swagger:route GET /files/download files downloadFileByPath
// swagger:route GET /files/:file-id files downloadFileByID
func ReadFileContentHandler(c *gin.Context, fileID string) {
	var err error

	instance := middlewares.GetInstance(c)

	path := c.Query("Path")

	// Path /files/download is handled specifically to download file
	// form their path
	var doc *vfs.FileDoc
	var disposition string
	if fileID == "" && path != "" {
		disposition = "attachment"
		doc, err = vfs.GetFileDocFromPath(instance, path)
	} else {
		disposition = "inline"
		doc, err = vfs.GetFileDoc(instance, fileID)
	}

	if err != nil {
		jsonapi.AbortWithError(c, wrapVfsError(err))
		return
	}

	err = vfs.ServeFileContent(instance, doc, disposition, c.Request, c.Writer)

	if err != nil {
		jsonapi.AbortWithError(c, wrapVfsError(err))
		return
	}
}

// TrashHandler handles all DELETE requests on /files/:file-id and
// moves the file or directory with the specified file-id to the
// trash.
//
// swagger:route DELETE /files/:file-id files trashFileOrDirectory
func TrashHandler(c *gin.Context) {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	dir, file, err := vfs.GetDirOrFileDoc(instance, fileID, true)
	if err != nil {
		jsonapi.AbortWithError(c, wrapVfsError(err))
		return
	}

	var data jsonapi.Object
	if dir != nil {
		data, err = vfs.TrashDir(instance, dir)
	} else {
		data, err = vfs.TrashFile(instance, file)
	}

	if err != nil {
		jsonapi.AbortWithError(c, wrapVfsError(err))
		return
	}

	jsonapi.Data(c, http.StatusOK, data, nil)
}

// ReadTrashFilesHandler handle GET requests on /files/trash and return the
// list of trashed files and directories
func ReadTrashFilesHandler(c *gin.Context) {
	instance := middlewares.GetInstance(c)

	trash, err := vfs.GetDirDoc(instance, vfs.TrashFolderID, true)
	if err != nil {
		jsonapi.AbortWithError(c, wrapVfsError(err))
		return
	}

	jsonapi.DataList(c, http.StatusOK, trash.Included(), nil)
}

// Routes sets the routing for the files service
func Routes(router *gin.RouterGroup) {
	// @TODO: get rid of this handler when switching to
	// echo/httprouterv2. This should ideally be:
	//
	//     router.HEAD("/download/:file-id", ReadFileContentFromIDHandler)
	//     router.HEAD("/download", ReadFileContentFromPathHandler)
	//     router.GET("/download", ReadFileContentFromPathHandler)
	//     router.GET("/download/:file-id", ReadFileContentFromIDHandler)
	//     router.GET("/metadata", ReadMetadataFromPathHandler)
	//     router.GET("/trash", ReadMetadataFromPathHandler)
	//     router.GET("/:file-id", ReadMetadataFromIDHanler)
	//
	router.HEAD("/download/:file-id", func(c *gin.Context) {
		ReadFileContentHandler(c, c.Param("file-id"))
	})
	router.GET("/:dl-meta-or-file-id/*file-id", func(c *gin.Context) {
		fileID := c.Param("file-id")[1:]
		ReadFileContentHandler(c, fileID)
	})
	router.GET("/:dl-meta-or-file-id", func(c *gin.Context) {
		dlMeta := c.Param("dl-meta-or-file-id")
		switch dlMeta {
		case "download":
			ReadFileContentHandler(c, "")
		case "metadata":
			ReadMetadataFromPathHandler(c)
		case "trash":
			ReadTrashFilesHandler(c)
		default:
			ReadMetadataFromIDHandler(c, dlMeta)
		}
	})

	router.POST("/", CreationHandler)
	router.POST("/:folder-id", CreationHandler)

	router.PATCH("/:file-id", ModificationHandler)
	router.PUT("/:file-id", OverwriteFileContentHandler)

	router.DELETE("/:file-id", TrashHandler)
}

// wrapVfsError returns a formatted error from a golang error emitted by the vfs
func wrapVfsError(err error) *jsonapi.Error {
	if jsonErr, isJSONApiError := err.(*jsonapi.Error); isJSONApiError {
		return jsonErr
	}
	if couchErr, isCouchErr := err.(*couchdb.Error); isCouchErr {
		return jsonapi.WrapCouchError(couchErr)
	}
	if os.IsExist(err) || err == vfs.ErrConflict {
		return &jsonapi.Error{
			Status: http.StatusConflict,
			Title:  "Conflict",
			Detail: err.Error(),
		}
	}
	if os.IsNotExist(err) {
		return jsonapi.NotFound(err)
	}
	switch err {
	case ErrDocTypeInvalid:
		return jsonapi.InvalidAttribute("type", err)
	case vfs.ErrParentDoesNotExist:
		return jsonapi.NotFound(err)
	case vfs.ErrForbiddenDocMove:
		return jsonapi.PreconditionFailed("folder-id", err)
	case vfs.ErrIllegalFilename:
		return jsonapi.InvalidParameter("name", err)
	case vfs.ErrIllegalTime:
		return jsonapi.InvalidParameter("UpdatedAt", err)
	case vfs.ErrInvalidHash:
		return jsonapi.PreconditionFailed("Content-MD5", err)
	case vfs.ErrContentLengthMismatch:
		return jsonapi.PreconditionFailed("Content-Length", err)
	case vfs.ErrFileInTrash:
		return jsonapi.BadRequest(err)
	case vfs.ErrNonAbsolutePath:
		return jsonapi.BadRequest(err)
	case vfs.ErrDirectoryNotEmpty:
		return jsonapi.BadRequest(err)
	}
	return jsonapi.InternalServerError(err)
}

func fileDocFromReq(c *gin.Context, name, folderID string, tags []string) (doc *vfs.FileDoc, err error) {
	header := c.Request.Header

	size, err := parseContentLength(header.Get("Content-Length"))
	if err != nil {
		jsonapi.AbortWithError(c, jsonapi.InvalidParameter("Content-Length", err))
		return
	}

	var md5Sum []byte
	if md5Str := header.Get("Content-MD5"); md5Str != "" {
		md5Sum, err = parseMD5Hash(md5Str)
	}
	if err != nil {
		jsonapi.AbortWithError(c, jsonapi.InvalidParameter("Content-MD5", err))
		return
	}

	executable := c.Query("Executable") == "true"
	mime, class := vfs.ExtractMimeAndClass(c.ContentType())
	doc, err = vfs.NewFileDoc(
		name,
		folderID,
		size,
		md5Sum,
		mime,
		class,
		executable,
		tags,
	)

	return
}

func checkIfMatch(c *gin.Context, rev string) error {
	ifMatch := c.Request.Header.Get("If-Match")
	revQuery := c.Query("rev")
	var wantedRev string
	if ifMatch != "" {
		wantedRev = ifMatch
	}
	if revQuery != "" && wantedRev == "" {
		wantedRev = revQuery
	}
	if wantedRev != "" && rev != wantedRev {
		return jsonapi.PreconditionFailed("If-Match", fmt.Errorf("Revision does not match."))
	}
	return nil
}

func parseMD5Hash(md5B64 string) ([]byte, error) {
	// Encoded md5 hash in base64 should at least have 22 caracters in
	// base64: 16*3/4 = 21+1/3
	//
	// The padding may add up to 2 characters (non useful). If we are
	// out of these boundaries we know we don't have a good hash and we
	// can bail immediatly.
	if len(md5B64) < 22 || len(md5B64) > 24 {
		return nil, fmt.Errorf("Given Content-MD5 is invalid")
	}

	md5Sum, err := base64.StdEncoding.DecodeString(md5B64)
	if err != nil || len(md5Sum) != 16 {
		return nil, fmt.Errorf("Given Content-MD5 is invalid")
	}

	return md5Sum, nil
}

func parseContentLength(contentLength string) (size int64, err error) {
	if contentLength == "" {
		size = -1
		return
	}

	size, err = strconv.ParseInt(contentLength, 10, 64)
	if err != nil {
		err = fmt.Errorf("Invalid content length")
	}
	return
}
