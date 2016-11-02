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

// DefaultContentType is used for files uploaded with no content-type
const DefaultContentType = "application/octet-stream"

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
	vfsC, err := getVfsContext(c)
	if err != nil {
		return
	}

	var doc jsonapi.Object
	switch c.Query("Type") {
	case fileType:
		doc, err = createFileHandler(c, vfsC)
	case folderType:
		doc, err = createDirectoryHandler(c, vfsC)
	default:
		err = ErrDocTypeInvalid
	}

	if err != nil {
		jsonapi.AbortWithError(c, WrapVfsError(err))
		return
	}

	jsonapi.Data(c, http.StatusCreated, doc, nil)
}

func createFileHandler(c *gin.Context, vfsC *vfs.Context) (doc *vfs.FileDoc, err error) {
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

	_, err = io.Copy(file, c.Request.Body)
	if err != nil {
		return
	}

	err = file.Close()
	if err != nil {
		return
	}

	return
}

func createDirectoryHandler(c *gin.Context, vfsC *vfs.Context) (doc *vfs.DirDoc, err error) {
	doc, err = vfs.NewDirDoc(
		c.Query("Name"),
		c.Param("folder-id"),
		strings.Split(c.Query("Tags"), TagSeparator),
		nil,
	)
	if err != nil {
		return
	}

	err = vfs.CreateDirectory(vfsC, doc)
	if err != nil {
		return
	}

	return
}

// OverwriteFileContentHandler handles PUT requests on /files/:file-id
// to overwrite the content of a file given its identifier.
//
// swagger:route PUT /files/:file-id files overwriteFileContent
func OverwriteFileContentHandler(c *gin.Context) {
	var err error

	vfsC, err := getVfsContext(c)
	if err != nil {
		return
	}

	var olddoc *vfs.FileDoc
	var newdoc *vfs.FileDoc

	olddoc, err = vfs.GetFileDoc(vfsC, c.Param("file-id"))
	if err != nil {
		jsonapi.AbortWithError(c, WrapVfsError(err))
		return
	}

	newdoc, err = fileDocFromReq(
		c,
		olddoc.Name,
		olddoc.FolderID,
		olddoc.Tags,
	)
	if err != nil {
		jsonapi.AbortWithError(c, WrapVfsError(err))
		return
	}

	if err = checkIfMatch(c.Request, olddoc.Rev()); err != nil {
		jsonapi.AbortWithError(c, WrapVfsError(err))
		return
	}

	file, err := vfs.CreateFile(vfsC, newdoc, olddoc)
	if err != nil {
		jsonapi.AbortWithError(c, WrapVfsError(err))
		return
	}

	_, err = io.Copy(file, c.Request.Body)
	if err != nil {
		jsonapi.AbortWithError(c, WrapVfsError(err))
		return
	}

	err = file.Close()
	if err != nil {
		jsonapi.AbortWithError(c, WrapVfsError(err))
		return
	}

	jsonapi.Data(c, http.StatusOK, newdoc, nil)
}

// ModificationHandler handles PATCH requests on /files/:file-id and
// /files/metadata.
//
// It can be used to modify the file or directory metadata, as well as
// moving and renaming it in the filesystem.
func ModificationHandler(c *gin.Context) {
	var err error

	vfsC, err := getVfsContext(c)
	if err != nil {
		jsonapi.AbortWithError(c, jsonapi.InternalServerError(err))
		return
	}

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

	var typ string
	var file *vfs.FileDoc
	var dir *vfs.DirDoc

	if fileID == "metadata" {
		typ, dir, file, err = vfs.GetDirOrFileDocFromPath(vfsC, c.Query("Path"), false)
	} else {
		typ, dir, file, err = vfs.GetDirOrFileDoc(vfsC, fileID, false)
	}

	if err != nil {
		jsonapi.AbortWithError(c, WrapVfsError(err))
		return
	}

	var doc couchdb.Doc
	switch typ {
	case vfs.DirType:
		doc = dir
	case vfs.FileType:
		doc = file
	}

	if err = checkIfMatch(c.Request, doc.Rev()); err != nil {
		jsonapi.AbortWithError(c, WrapVfsError(err))
		return
	}

	var data jsonapi.Object
	if fileDoc, ok := doc.(*vfs.FileDoc); ok {
		data, err = vfs.ModifyFileMetadata(vfsC, fileDoc, patch)
	} else if dirDoc, ok := doc.(*vfs.DirDoc); ok {
		data, err = vfs.ModifyDirectoryMetadata(vfsC, dirDoc, patch)
	}

	if err != nil {
		jsonapi.AbortWithError(c, WrapVfsError(err))
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

	vfsC, err := getVfsContext(c)
	if err != nil {
		return
	}

	typ, dir, file, err := vfs.GetDirOrFileDoc(vfsC, fileID, true)
	if err != nil {
		jsonapi.AbortWithError(c, WrapVfsError(err))
		return
	}

	var data jsonapi.Object
	switch typ {
	case vfs.DirType:
		data = dir
	case vfs.FileType:
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

	vfsC, err := getVfsContext(c)
	if err != nil {
		return
	}

	typ, dir, file, err := vfs.GetDirOrFileDocFromPath(vfsC, c.Query("Path"), true)
	if err != nil {
		jsonapi.AbortWithError(c, WrapVfsError(err))
		return
	}

	var data jsonapi.Object
	switch typ {
	case vfs.DirType:
		data = dir
	case vfs.FileType:
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

	vfsC, err := getVfsContext(c)
	if err != nil {
		return
	}

	path := c.Query("Path")

	// Path /files/download is handled specifically to download file
	// form their path
	var doc *vfs.FileDoc
	var disposition string
	if fileID == "" && path != "" {
		disposition = "attachment"
		doc, err = vfs.GetFileDocFromPath(vfsC, path)
	} else {
		disposition = "inline"
		doc, err = vfs.GetFileDoc(vfsC, fileID)
	}

	if err != nil {
		jsonapi.AbortWithError(c, WrapVfsError(err))
		return
	}

	err = vfs.ServeFileContent(vfsC, doc, disposition, c.Request, c.Writer)

	if err != nil {
		jsonapi.AbortWithError(c, WrapVfsError(err))
		return
	}
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
		if dlMeta == "download" {
			ReadFileContentHandler(c, "")
		} else if dlMeta == "metadata" {
			ReadMetadataFromPathHandler(c)
		} else {
			ReadMetadataFromIDHandler(c, dlMeta)
		}
	})

	router.POST("/", CreationHandler)
	router.POST("/:folder-id", CreationHandler)

	router.PATCH("/:file-id", ModificationHandler)
	router.PUT("/:file-id", OverwriteFileContentHandler)
}

// WrapVfsError returns a formatted error from a golang error emitted by the vfs
func WrapVfsError(err error) *jsonapi.Error {
	if jsonErr, isJSONApiError := err.(*jsonapi.Error); isJSONApiError {
		return jsonErr
	}
	if couchErr, isCouchErr := err.(*couchdb.Error); isCouchErr {
		return jsonapi.WrapCouchError(couchErr)
	}
	if os.IsExist(err) {
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
	}
	return jsonapi.InternalServerError(err)
}

func getVfsContext(c *gin.Context) (*vfs.Context, error) {
	instance := middlewares.GetInstance(c)
	vfsC, err := instance.GetVFSContext()
	if err != nil {
		jsonapi.AbortWithError(c, jsonapi.InternalServerError(err))
		return nil, err
	}
	return vfsC, nil
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
	mime, class := extractMimeAndClass(c.ContentType())
	doc, err = vfs.NewFileDoc(
		name,
		folderID,
		size,
		md5Sum,
		mime,
		class,
		executable,
		tags,
		nil,
	)

	return
}

func checkIfMatch(req *http.Request, rev string) error {
	ifMatch := req.Header.Get("If-Match")
	if ifMatch != "" && rev != ifMatch {
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

func extractMimeAndClass(contentType string) (mime, class string) {
	if contentType == "" {
		contentType = DefaultContentType
	}

	charsetIndex := strings.Index(contentType, ";")
	if charsetIndex >= 0 {
		mime = contentType[:charsetIndex]
	} else {
		mime = contentType
	}

	// @TODO improve for specific mime types
	slashIndex := strings.Index(contentType, "/")
	if slashIndex >= 0 {
		class = contentType[:slashIndex]
	} else {
		class = contentType
	}

	return
}
