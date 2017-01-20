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
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

// TagSeparator is the character separating tags
const TagSeparator = ","

// ErrDocTypeInvalid is used when the document type sent is not
// recognized
var ErrDocTypeInvalid = errors.New("Invalid document type")

// CreationHandler handle all POST requests on /files/:dir-id
// aiming at creating a new document in the FS. Given the Type
// parameter of the request, it will either upload a new file or
// create a new directory.
func CreationHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	var doc jsonapi.Object
	var err error
	switch c.QueryParam("Type") {
	case consts.FileType:
		doc, err = createFileHandler(c, instance)
	case consts.DirType:
		doc, err = createDirHandler(c, instance)
	default:
		err = ErrDocTypeInvalid
	}

	if err != nil {
		return wrapVfsError(err)
	}

	return jsonapi.Data(c, http.StatusCreated, doc, nil)
}

func createFileHandler(c echo.Context, vfsC vfs.Context) (doc *vfs.FileDoc, err error) {
	tags := strings.Split(c.QueryParam("Tags"), TagSeparator)

	dirID := c.Param("dir-id")
	name := c.QueryParam("Name")
	doc, err = fileDocFromReq(c, name, dirID, tags)
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

	_, err = io.Copy(file, c.Request().Body)
	return
}

func createDirHandler(c echo.Context, vfsC vfs.Context) (*vfs.DirDoc, error) {
	path := c.QueryParam("Path")
	tags := strings.Split(c.QueryParam("Tags"), TagSeparator)

	if path != "" {
		if c.QueryParam("Recursive") == "true" {
			return vfs.MkdirAll(vfsC, path, tags)
		}
		return vfs.Mkdir(vfsC, path, tags)
	}

	dirID := c.Param("dir-id")
	name := c.QueryParam("Name")
	doc, err := vfs.NewDirDoc(name, dirID, tags, nil)
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
func OverwriteFileContentHandler(c echo.Context) (err error) {
	var instance = middlewares.GetInstance(c)
	var olddoc *vfs.FileDoc
	var newdoc *vfs.FileDoc

	olddoc, err = vfs.GetFileDoc(instance, c.Param("file-id"))
	if err != nil {
		return wrapVfsError(err)
	}

	newdoc, err = fileDocFromReq(
		c,
		olddoc.Name,
		olddoc.DirID,
		olddoc.Tags,
	)
	if err != nil {
		return wrapVfsError(err)
	}

	if err = checkIfMatch(c, olddoc.Rev()); err != nil {
		return wrapVfsError(err)
	}

	file, err := vfs.CreateFile(instance, newdoc, olddoc)
	if err != nil {
		return wrapVfsError(err)
	}

	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
		if err != nil {
			err = wrapVfsError(err)
			return
		}
		err = jsonapi.Data(c, http.StatusOK, newdoc, nil)
	}()

	_, err = io.Copy(file, c.Request().Body)
	return
}

// ModifyMetadataByIDHandler handles PATCH requests on /files/:file-id
//
// It can be used to modify the file or directory metadata, as well as
// moving and renaming it in the filesystem.
func ModifyMetadataByIDHandler(c echo.Context) error {
	patch, err := getPatch(c)
	if err != nil {
		return wrapVfsError(err)
	}

	instance := middlewares.GetInstance(c)
	dir, file, err := vfs.GetDirOrFileDoc(instance, c.Param("file-id"), false)
	if err != nil {
		return wrapVfsError(err)
	}

	return applyPatch(c, instance, patch, dir, file)
}

// ModifyMetadataByPathHandler handles PATCH requests on /files/:file-id
//
// It can be used to modify the file or directory metadata, as well as
// moving and renaming it in the filesystem.
func ModifyMetadataByPathHandler(c echo.Context) error {
	patch, err := getPatch(c)
	if err != nil {
		return wrapVfsError(err)
	}

	instance := middlewares.GetInstance(c)
	dir, file, err := vfs.GetDirOrFileDocFromPath(instance, c.QueryParam("Path"), false)
	if err != nil {
		return wrapVfsError(err)
	}

	return applyPatch(c, instance, patch, dir, file)
}

func getPatch(c echo.Context) (*vfs.DocPatch, error) {
	var patch vfs.DocPatch

	obj, err := jsonapi.Bind(c.Request(), &patch)
	if err != nil {
		return nil, jsonapi.BadJSON()
	}

	if rel, ok := obj.GetRelationship("parent"); ok {
		rid, ok := rel.ResourceIdentifier()
		if !ok {
			return nil, jsonapi.BadJSON()
		}
		patch.DirID = &rid.ID
	}

	return &patch, nil
}

func applyPatch(c echo.Context, instance *instance.Instance, patch *vfs.DocPatch, dir *vfs.DirDoc, file *vfs.FileDoc) error {
	var doc couchdb.Doc
	if dir != nil {
		doc = dir
	} else {
		doc = file
	}

	if err := checkIfMatch(c, doc.Rev()); err != nil {
		return wrapVfsError(err)
	}

	var data jsonapi.Object
	var err error
	if fileDoc, ok := doc.(*vfs.FileDoc); ok {
		data, err = vfs.ModifyFileMetadata(instance, fileDoc, patch)
	} else if dirDoc, ok := doc.(*vfs.DirDoc); ok {
		data, err = vfs.ModifyDirMetadata(instance, dirDoc, patch)
	}

	if err != nil {
		return wrapVfsError(err)
	}

	return jsonapi.Data(c, http.StatusOK, data, nil)
}

// ReadMetadataFromIDHandler handles all GET requests on /files/:file-
// id aiming at getting file metadata from its path.
func ReadMetadataFromIDHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	dir, file, err := vfs.GetDirOrFileDoc(instance, fileID, true)
	if err != nil {
		return wrapVfsError(err)
	}

	var data jsonapi.Object
	if dir != nil {
		data = dir
	} else {
		data = file
	}

	return jsonapi.Data(c, http.StatusOK, data, nil)
}

// ReadMetadataFromPathHandler handles all GET requests on
// /files/metadata aiming at getting file metadata from its path.
func ReadMetadataFromPathHandler(c echo.Context) error {
	var err error

	instance := middlewares.GetInstance(c)

	dir, file, err := vfs.GetDirOrFileDocFromPath(instance, c.QueryParam("Path"), true)
	if err != nil {
		return wrapVfsError(err)
	}

	var data jsonapi.Object
	if dir != nil {
		data = dir
	} else {
		data = file
	}

	return jsonapi.Data(c, http.StatusOK, data, nil)
}

// ReadFileContentFromIDHandler handles all GET requests on /files/:file-id
// aiming at downloading a file given its ID. It serves the file in inline
// mode.
func ReadFileContentFromIDHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	doc, err := vfs.GetFileDoc(instance, c.Param("file-id"))
	if err != nil {
		return wrapVfsError(err)
	}

	err = vfs.ServeFileContent(instance, doc, "inline", c.Request(), c.Response())
	if err != nil {
		return wrapVfsError(err)
	}

	return nil
}

// ReadFileContentFromPathHandler handles all GET request on /files/download
// aiming at downloading a file given its path. It serves the file in in
// attachment mode.
func ReadFileContentFromPathHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	path := c.QueryParam("Path")
	doc, err := vfs.GetFileDocFromPath(instance, path)
	if err != nil {
		return wrapVfsError(err)
	}

	err = vfs.ServeFileContent(instance, doc, "attachment", c.Request(), c.Response())
	if err != nil {
		return wrapVfsError(err)
	}

	return nil
}

// ArchiveHandler handles requests to /files/archive and creates on the fly
// zip archives with the given files and folders.
func ArchiveHandler(c echo.Context) error {
	archive := &vfs.Archive{}
	if _, err := jsonapi.Bind(c.Request(), archive); err != nil {
		return err
	}
	if len(archive.Files) == 0 {
		return c.JSON(http.StatusBadRequest, "Can't create an archive with no files")
	}
	if archive.Name == "" {
		archive.Name = "archive"
	}

	instance := middlewares.GetInstance(c)
	return archive.Serve(instance, c.Response())
}

// TrashHandler handles all DELETE requests on /files/:file-id and
// moves the file or directory with the specified file-id to the
// trash.
func TrashHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	dir, file, err := vfs.GetDirOrFileDoc(instance, fileID, true)
	if err != nil {
		return wrapVfsError(err)
	}

	var data jsonapi.Object
	if dir != nil {
		data, err = vfs.TrashDir(instance, dir)
	} else {
		data, err = vfs.TrashFile(instance, file)
	}

	if err != nil {
		return wrapVfsError(err)
	}

	return jsonapi.Data(c, http.StatusOK, data, nil)
}

// ReadTrashFilesHandler handle GET requests on /files/trash and return the
// list of trashed files and directories
func ReadTrashFilesHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	trash, err := vfs.GetDirDoc(instance, consts.TrashDirID, true)
	if err != nil {
		return wrapVfsError(err)
	}

	return jsonapi.DataList(c, http.StatusOK, trash.Included(), nil)
}

// RestoreTrashFileHandler handle POST requests on /files/trash/file-id and
// can be used to restore a file or directory from the trash.
func RestoreTrashFileHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	dir, file, err := vfs.GetDirOrFileDoc(instance, fileID, true)
	if err != nil {
		return wrapVfsError(err)
	}

	var data jsonapi.Object
	if dir != nil {
		data, err = vfs.RestoreDir(instance, dir)
	} else {
		data, err = vfs.RestoreFile(instance, file)
	}

	if err != nil {
		return wrapVfsError(err)
	}

	return jsonapi.Data(c, http.StatusOK, data, nil)
}

// ClearTrashHandler handles DELETE request to clear the trash
func ClearTrashHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	trash, err := vfs.GetDirDoc(instance, consts.TrashDirID, true)
	if err != nil {
		return wrapVfsError(err)
	}
	err = vfs.DestroyDirContent(instance, trash)
	if err != nil {
		return wrapVfsError(err)
	}

	return c.NoContent(204)
}

// DestroyFileHandler handles DELETE request to clear one element from the trash
func DestroyFileHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	dir, file, err := vfs.GetDirOrFileDoc(instance, fileID, true)
	if err != nil {
		return wrapVfsError(err)
	}

	if dir != nil {
		err = vfs.DestroyDirAndContent(instance, dir)
	} else {
		err = vfs.DestroyFile(instance, file)
	}
	if err != nil {
		return wrapVfsError(err)
	}

	return c.NoContent(204)
}

// Routes sets the routing for the files service
func Routes(router *echo.Group) {
	router.HEAD("/download", ReadFileContentFromPathHandler)
	router.GET("/download", ReadFileContentFromPathHandler)
	router.HEAD("/download/:file-id", ReadFileContentFromIDHandler)
	router.GET("/download/:file-id", ReadFileContentFromIDHandler)

	router.GET("/metadata", ReadMetadataFromPathHandler)
	router.GET("/:file-id", ReadMetadataFromIDHandler)

	router.PATCH("/metadata", ModifyMetadataByPathHandler)
	router.PATCH("/:file-id", ModifyMetadataByIDHandler)

	router.POST("/", CreationHandler)
	router.POST("/:dir-id", CreationHandler)
	router.PUT("/:file-id", OverwriteFileContentHandler)

	router.POST("/archive", ArchiveHandler)

	router.GET("/trash", ReadTrashFilesHandler)
	router.DELETE("/trash", ClearTrashHandler)

	router.POST("/trash/:file-id", RestoreTrashFileHandler)
	router.DELETE("/trash/:file-id", DestroyFileHandler)

	router.DELETE("/:file-id", TrashHandler)
}

// wrapVfsError returns a formatted error from a golang error emitted by the vfs
func wrapVfsError(err error) error {
	switch err {
	case ErrDocTypeInvalid:
		return jsonapi.InvalidAttribute("type", err)
	case vfs.ErrParentDoesNotExist:
		return jsonapi.NotFound(err)
	case vfs.ErrForbiddenDocMove:
		return jsonapi.PreconditionFailed("dir-id", err)
	case vfs.ErrIllegalFilename:
		return jsonapi.InvalidParameter("name", err)
	case vfs.ErrIllegalTime:
		return jsonapi.InvalidParameter("UpdatedAt", err)
	case vfs.ErrInvalidHash:
		return jsonapi.PreconditionFailed("Content-MD5", err)
	case vfs.ErrContentLengthMismatch:
		return jsonapi.PreconditionFailed("Content-Length", err)
	case vfs.ErrConflict:
		return jsonapi.Conflict(err)
	case vfs.ErrFileInTrash:
		return jsonapi.BadRequest(err)
	case vfs.ErrNonAbsolutePath:
		return jsonapi.BadRequest(err)
	case vfs.ErrDirNotEmpty:
		return jsonapi.BadRequest(err)
	}
	return err
}

func fileDocFromReq(c echo.Context, name, dirID string, tags []string) (doc *vfs.FileDoc, err error) {
	header := c.Request().Header

	size, err := parseContentLength(header.Get("Content-Length"))
	if err != nil {
		err = jsonapi.InvalidParameter("Content-Length", err)
		return
	}

	var md5Sum []byte
	if md5Str := header.Get("Content-MD5"); md5Str != "" {
		md5Sum, err = parseMD5Hash(md5Str)
	}
	if err != nil {
		err = jsonapi.InvalidParameter("Content-MD5", err)
		return
	}

	executable := c.QueryParam("Executable") == "true"
	contentType := header.Get("Content-Type")
	mime, class := vfs.ExtractMimeAndClass(contentType)
	doc, err = vfs.NewFileDoc(
		name,
		dirID,
		size,
		md5Sum,
		mime,
		class,
		executable,
		tags,
	)

	return
}

func checkIfMatch(c echo.Context, rev string) error {
	ifMatch := c.Request().Header.Get("If-Match")
	revQuery := c.QueryParam("rev")
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
