// Package files is the HTTP frontend of the vfs package. It exposes
// an HTTP api to manipulate the filesystem and offer all the
// possibilities given by the vfs.
package files

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	pkgperm "github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
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
		doc, err = createFileHandler(c, instance.VFS())
	case consts.DirType:
		doc, err = createDirHandler(c, instance.VFS())
	default:
		err = ErrDocTypeInvalid
	}

	if err != nil {
		return wrapVfsError(err)
	}

	return jsonapi.Data(c, http.StatusCreated, doc, nil)
}

func createFileHandler(c echo.Context, fs vfs.VFS) (f *file, err error) {
	tags := strings.Split(c.QueryParam("Tags"), TagSeparator)

	dirID := c.Param("dir-id")
	name := c.QueryParam("Name")
	var doc *vfs.FileDoc
	doc, err = FileDocFromReq(c, name, dirID, tags)
	if err != nil {
		return
	}

	err = checkPerm(c, "POST", nil, doc)
	if err != nil {
		return
	}

	file, err := fs.CreateFile(doc, nil)
	if err != nil {
		return
	}

	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	_, err = io.Copy(file, c.Request().Body)
	if err != nil {
		return
	}
	instance := middlewares.GetInstance(c)
	f = newFile(doc, instance, middlewares.GetSessionID(c))
	return
}

func createDirHandler(c echo.Context, fs vfs.VFS) (*dir, error) {
	path := c.QueryParam("Path")
	tags := utils.SplitTrimString(c.QueryParam("Tags"), TagSeparator)

	var doc *vfs.DirDoc
	var err error
	if path != "" {
		if c.QueryParam("Recursive") == "true" {
			doc, err = vfs.MkdirAll(fs, path, tags)
		} else {
			doc, err = vfs.Mkdir(fs, path, tags)
		}
		if err != nil {
			return nil, err
		}
		return newDir(doc), nil
	}

	dirID := c.Param("dir-id")
	name := c.QueryParam("Name")
	doc, err = vfs.NewDirDoc(fs, name, dirID, tags)
	if err != nil {
		return nil, err
	}
	if date := c.Request().Header.Get("Date"); date != "" {
		if t, err2 := time.Parse(time.RFC1123, date); err2 == nil {
			doc.CreatedAt = t
			doc.UpdatedAt = t
		}
	}

	err = checkPerm(c, "POST", doc, nil)
	if err != nil {
		return nil, err
	}

	if err = fs.CreateDir(doc); err != nil {
		return nil, err
	}

	return newDir(doc), nil
}

// OverwriteFileContentHandler handles PUT requests on /files/:file-id
// to overwrite the content of a file given its identifier.
func OverwriteFileContentHandler(c echo.Context) (err error) {
	var instance = middlewares.GetInstance(c)
	var olddoc *vfs.FileDoc
	var newdoc *vfs.FileDoc

	olddoc, err = instance.VFS().FileByID(c.Param("file-id"))
	if err != nil {
		return wrapVfsError(err)
	}

	newdoc, err = FileDocFromReq(
		c,
		olddoc.DocName,
		olddoc.DirID,
		olddoc.Tags,
	)
	if err != nil {
		return wrapVfsError(err)
	}

	newdoc.ReferencedBy = olddoc.ReferencedBy

	if err = CheckIfMatch(c, olddoc.Rev()); err != nil {
		return wrapVfsError(err)
	}

	err = checkPerm(c, permissions.PUT, nil, olddoc)
	if err != nil {
		return
	}

	err = checkPerm(c, permissions.PUT, nil, newdoc)
	if err != nil {
		return
	}

	file, err := instance.VFS().CreateFile(newdoc, olddoc)
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
		err = fileData(c, http.StatusOK, newdoc, false)
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
	dir, file, err := instance.VFS().DirOrFileByID(c.Param("file-id"))
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
	dir, file, err := instance.VFS().DirOrFileByPath(c.QueryParam("Path"))
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

	patch.RestorePath = nil
	return &patch, nil
}

func applyPatch(c echo.Context, instance *instance.Instance, patch *vfs.DocPatch, dir *vfs.DirDoc, file *vfs.FileDoc) error {
	var rev string
	if dir != nil {
		rev = dir.Rev()
	} else {
		rev = file.Rev()
	}

	if err := CheckIfMatch(c, rev); err != nil {
		return wrapVfsError(err)
	}

	if err := checkPerm(c, permissions.PATCH, dir, file); err != nil {
		return err
	}

	if dir != nil {
		doc, err := vfs.ModifyDirMetadata(instance.VFS(), dir, patch)
		if err != nil {
			return wrapVfsError(err)
		}
		return dirData(c, http.StatusOK, doc)
	}

	doc, err := vfs.ModifyFileMetadata(instance.VFS(), file, patch)
	if err != nil {
		return wrapVfsError(err)
	}
	return fileData(c, http.StatusOK, doc, false)
}

// ReadMetadataFromIDHandler handles all GET requests on /files/:file-
// id aiming at getting file metadata from its id.
func ReadMetadataFromIDHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	dir, file, err := instance.VFS().DirOrFileByID(fileID)
	if err != nil {
		return wrapVfsError(err)
	}

	if err := checkPerm(c, permissions.GET, dir, file); err != nil {
		return err
	}

	if dir != nil {
		return dirData(c, http.StatusOK, dir)
	}
	return fileData(c, http.StatusOK, file, true)
}

// GetChildrenHandler returns a list of children of a folder
func GetChildrenHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	dir, file, err := instance.VFS().DirOrFileByID(fileID)
	if err != nil {
		return wrapVfsError(err)
	}

	if file != nil {
		return jsonapi.NewError(400, "cant read children of file "+fileID)
	}

	return dirDataList(c, http.StatusOK, dir)
}

// ReadMetadataFromPathHandler handles all GET requests on
// /files/metadata aiming at getting file metadata from its path.
func ReadMetadataFromPathHandler(c echo.Context) error {
	var err error

	instance := middlewares.GetInstance(c)

	dir, file, err := instance.VFS().DirOrFileByPath(c.QueryParam("Path"))
	if err != nil {
		return wrapVfsError(err)
	}

	if err := checkPerm(c, permissions.GET, dir, file); err != nil {
		return err
	}

	if dir != nil {
		return dirData(c, http.StatusOK, dir)
	}
	return fileData(c, http.StatusOK, file, true)
}

// ReadFileContentFromIDHandler handles all GET requests on /files/:file-id
// aiming at downloading a file given its ID. It serves the file in inline
// mode.
func ReadFileContentFromIDHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	doc, err := instance.VFS().FileByID(c.Param("file-id"))
	if err != nil {
		return wrapVfsError(err)
	}

	return sendFile(c, instance, doc, true)
}

// HeadDirOrFile handles HEAD requests on directory or file to check their
// existence
func HeadDirOrFile(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	dir, file, err := instance.VFS().DirOrFileByID(c.Param("file-id"))
	if err != nil {
		return wrapVfsError(err)
	}

	if dir != nil {
		err = checkPerm(c, permissions.GET, dir, nil)
	} else {
		err = checkPerm(c, permissions.GET, nil, file)
	}
	if err != nil {
		return err
	}

	return nil
}

// ThumbnailHandler serves thumbnails of the images/photos.
//
// In order to allow browsers applications to download thumbnails directly via
// an <img> tag, this handler can handle a secret directly inside the URL. This
// secret is only checked for a request send without an "Authorization" header
// but with a valid session cookie.
func ThumbnailHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")
	format := c.Param("format")
	secret := c.Param("secret")

	session, isLoggedIn := middlewares.GetSession(c)
	authHeader := c.Request().Header.Get("Authorization")

	hasSecureLinkSecret := false
	if isLoggedIn && authHeader == "" {
		hasSecureLinkSecret = secret != "" &&
			vfs.VerifySecureLinkSecret(instance.SessionSecret, secret, fileID, session.ID())
		if !hasSecureLinkSecret {
			return jsonapi.NewError(http.StatusUnauthorized, "Wrong download secret")
		}
	}

	doc, err := instance.VFS().FileByID(fileID)
	if err != nil {
		return wrapVfsError(err)
	}
	if !hasSecureLinkSecret {
		err = checkPerm(c, permissions.GET, nil, doc)
		if err != nil {
			return err
		}
	}

	fs := instance.ThumbsFS()
	return fs.ServeThumbContent(c.Response(), c.Request(), doc, format)
}

func sendFile(c echo.Context, instance *instance.Instance, doc *vfs.FileDoc, checkPermission bool) error {
	if checkPermission {
		err := checkPerm(c, permissions.GET, nil, doc)
		if err != nil {
			return err
		}
	}

	disposition := "inline"
	if c.QueryParam("Dl") == "1" {
		disposition = "attachment"
	} else if !checkPermission {
		// Allow some files to be displayed by the browser in the client-side apps
		if doc.Mime == "text/plain" || doc.Class == "image" || doc.Class == "audio" || doc.Class == "video" || doc.Mime == "application/pdf" {
			c.Response().Header().Del(echo.HeaderXFrameOptions)
		}
	}

	err := vfs.ServeFileContent(instance.VFS(), doc, disposition, c.Request(), c.Response())
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
	doc, err := instance.VFS().FileByPath(c.QueryParam("Path"))
	if err != nil {
		return wrapVfsError(err)
	}
	return sendFile(c, instance, doc, true)
}

// ArchiveDownloadCreateHandler handles requests to /files/archive and stores the
// paremeters with a secret to be used in download handler below.s
func ArchiveDownloadCreateHandler(c echo.Context) error {
	archive := &vfs.Archive{}
	if _, err := jsonapi.Bind(c.Request(), archive); err != nil {
		return err
	}
	if len(archive.Files) == 0 && len(archive.IDs) == 0 {
		return c.JSON(http.StatusBadRequest, "Can't create an archive with no files")
	}
	if strings.Contains(archive.Name, "/") {
		return c.JSON(http.StatusBadRequest, "The archive filename can't contain a /")
	}
	if archive.Name == "" {
		archive.Name = "archive"
	}
	instance := middlewares.GetInstance(c)

	entries, err := archive.GetEntries(instance.VFS())
	if err != nil {
		return wrapVfsError(err)
	}

	for _, e := range entries {
		err = checkPerm(c, permissions.GET, e.Dir, e.File)
		if err != nil {
			return err
		}
	}

	// if accept header is application/zip, send the archive immediately
	if c.Request().Header.Get("Accept") == "application/zip" {
		return archive.Serve(instance.VFS(), c.Response())
	}

	secret, err := vfs.GetStore().AddArchive(instance.Domain, archive)
	if err != nil {
		return wrapVfsError(err)
	}
	archive.Secret = secret

	fakeName := url.PathEscape(archive.Name)

	links := &jsonapi.LinksList{
		Related: "/files/archive/" + secret + "/" + fakeName + ".zip",
	}

	return jsonapi.Data(c, http.StatusOK, &apiArchive{archive}, links)
}

// FileDownloadCreateHandler stores the required path into a secret
// usable for download handler below.
func FileDownloadCreateHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	var doc *vfs.FileDoc
	var err error

	if path := c.QueryParam("Path"); path != "" {
		doc, err = instance.VFS().FileByPath(path)
	} else if id := c.QueryParam("Id"); id != "" {
		doc, err = instance.VFS().FileByID(id)
	} else {
		return jsonapi.BadRequest(errors.New("Missing Id or Path query parameter"))
	}
	if err != nil {
		return wrapVfsError(err)
	}

	err = checkPerm(c, permissions.GET, nil, doc)
	if err != nil {
		return err
	}

	return fileData(c, http.StatusOK, doc, true)
}

// ArchiveDownloadHandler handles requests to /files/archive/:secret/whatever.zip
// and creates on the fly zip archive from the parameters linked to secret.
func ArchiveDownloadHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	secret := c.Param("secret")
	archive, err := vfs.GetStore().GetArchive(instance.Domain, secret)
	if err != nil {
		return wrapVfsError(err)
	}
	if archive == nil {
		return jsonapi.NewError(http.StatusBadRequest, "Wrong download token")
	}
	return archive.Serve(instance.VFS(), c.Response())
}

// FileDownloadHandler send a file that have previously be defined
// through FileDownloadCreateHandler
func FileDownloadHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	secret := c.Param("secret")
	fileID := c.Param("file-id")
	var sessionID string
	if session, ok := middlewares.GetSession(c); ok {
		sessionID = session.ID()
	}
	if !vfs.VerifySecureLinkSecret(instance.SessionSecret, secret, fileID, sessionID) {
		return jsonapi.NewError(http.StatusBadRequest, "Wrong download token")
	}
	doc, err := instance.VFS().FileByID(fileID)
	if err != nil {
		return wrapVfsError(err)
	}
	return sendFile(c, instance, doc, false)
}

// TrashHandler handles all DELETE requests on /files/:file-id and
// moves the file or directory with the specified file-id to the
// trash.
func TrashHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	dir, file, err := instance.VFS().DirOrFileByID(fileID)
	if err != nil {
		return wrapVfsError(err)
	}

	err = checkPerm(c, permissions.PUT, dir, file)
	if err != nil {
		return err
	}

	var rev string
	if dir != nil {
		rev = dir.Rev()
	} else {
		rev = file.Rev()
	}

	if err := CheckIfMatch(c, rev); err != nil {
		return wrapVfsError(err)
	}

	if dir != nil {
		doc, errt := vfs.TrashDir(instance.VFS(), dir)
		if errt != nil {
			return wrapVfsError(errt)
		}
		return dirData(c, http.StatusOK, doc)
	}

	doc, errt := vfs.TrashFile(instance.VFS(), file)
	if errt != nil {
		return wrapVfsError(errt)
	}
	return fileData(c, http.StatusOK, doc, false)
}

// ReadTrashFilesHandler handle GET requests on /files/trash and return the
// list of trashed files and directories
func ReadTrashFilesHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	trash, err := instance.VFS().DirByID(consts.TrashDirID)
	if err != nil {
		return wrapVfsError(err)
	}

	err = checkPerm(c, permissions.GET, trash, nil)
	if err != nil {
		return err
	}

	return dirDataList(c, http.StatusOK, trash)
}

// RestoreTrashFileHandler handle POST requests on /files/trash/file-id and
// can be used to restore a file or directory from the trash.
func RestoreTrashFileHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	dir, file, err := instance.VFS().DirOrFileByID(fileID)
	if err != nil {
		return wrapVfsError(err)
	}

	err = checkPerm(c, permissions.PUT, dir, file)
	if err != nil {
		return err
	}

	if dir != nil {
		doc, errt := vfs.RestoreDir(instance.VFS(), dir)
		if errt != nil {
			return wrapVfsError(errt)
		}
		return dirData(c, http.StatusOK, doc)
	}

	doc, errt := vfs.RestoreFile(instance.VFS(), file)
	if errt != nil {
		return wrapVfsError(errt)
	}
	return fileData(c, http.StatusOK, doc, false)
}

// ClearTrashHandler handles DELETE request to clear the trash
func ClearTrashHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	trash, err := instance.VFS().DirByID(consts.TrashDirID)
	if err != nil {
		return wrapVfsError(err)
	}

	err = checkPerm(c, permissions.DELETE, trash, nil)
	if err != nil {
		return err
	}

	err = instance.VFS().DestroyDirContent(trash)
	if err != nil {
		return wrapVfsError(err)
	}

	return c.NoContent(204)
}

// DestroyFileHandler handles DELETE request to clear one element from the trash
func DestroyFileHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	dir, file, err := instance.VFS().DirOrFileByID(fileID)
	if err != nil {
		return wrapVfsError(err)
	}

	err = checkPerm(c, permissions.DELETE, dir, file)
	if err != nil {
		return err
	}

	var rev string
	if dir != nil {
		rev = dir.Rev()
	} else {
		rev = file.Rev()
	}

	if err = CheckIfMatch(c, rev); err != nil {
		return wrapVfsError(err)
	}

	if dir != nil {
		err = instance.VFS().DestroyDirAndContent(dir)
	} else {
		err = instance.VFS().DestroyFile(file)
	}
	if err != nil {
		return wrapVfsError(err)
	}

	return c.NoContent(204)
}

const maxMangoLimit = 100

// FindFilesMango is the route POST /files/_find
// used to retrieve files and their metadata from a mango query.
func FindFilesMango(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	sessionID := middlewares.GetSessionID(c)
	var findRequest map[string]interface{}

	if err := json.NewDecoder(c.Request().Body).Decode(&findRequest); err != nil {
		return jsonapi.NewError(http.StatusBadRequest, err)
	}

	if err := permissions.AllowWholeType(c, permissions.GET, consts.Files); err != nil {
		return err
	}

	// drop the fields, they can cause issues if not properly manipulated
	// TODO : optimization potential, necessary fields so far are class & type
	delete(findRequest, "fields")

	limit, hasLimit := findRequest["limit"].(float64)
	if !hasLimit || limit > maxMangoLimit {
		limit = 100
	}
	skip := 0
	skipF64, hasSkip := findRequest["skip"].(float64)
	if hasSkip {
		skip = int(skipF64)
	}

	// add 1 so we know if there is more.
	findRequest["limit"] = limit + 1

	var results []vfs.DirOrFileDoc
	err := couchdb.FindDocsRaw(instance, consts.Files, &findRequest, &results)
	if err != nil {
		return err
	}

	var total int
	if len(results) > int(limit) {
		total = math.MaxInt32 - 1          // we dont know the actual number
		results = results[:len(results)-1] // loose the last item
	} else {
		total = skip + len(results) // let the client know its done.
	}

	out := make([]jsonapi.Object, len(results))
	for i, dof := range results {
		d, f := dof.Refine()
		if d != nil {
			out[i] = newDir(d)
		} else {
			out[i] = newFile(f, instance, sessionID)
		}
	}

	return jsonapi.DataListWithTotal(c, http.StatusOK, total, out, nil)
}

// Routes sets the routing for the files service
func Routes(router *echo.Group) {
	router.HEAD("/download", ReadFileContentFromPathHandler)
	router.GET("/download", ReadFileContentFromPathHandler)
	router.HEAD("/download/:file-id", ReadFileContentFromIDHandler)
	router.GET("/download/:file-id", ReadFileContentFromIDHandler)

	router.POST("/_find", FindFilesMango)

	router.HEAD("/:file-id", HeadDirOrFile)
	router.GET("/metadata", ReadMetadataFromPathHandler)
	router.GET("/:file-id", ReadMetadataFromIDHandler)
	router.GET("/:file-id/relationships/contents", GetChildrenHandler)

	router.PATCH("/metadata", ModifyMetadataByPathHandler)
	router.PATCH("/:file-id", ModifyMetadataByIDHandler)

	router.POST("/", CreationHandler)
	router.POST("/:dir-id", CreationHandler)
	router.PUT("/:file-id", OverwriteFileContentHandler)

	router.GET("/:file-id/thumbnails/:format", ThumbnailHandler)
	router.GET("/:file-id/thumbnails/:format/:secret", ThumbnailHandler)

	router.POST("/archive", ArchiveDownloadCreateHandler)
	router.GET("/archive/:secret/:fake-name", ArchiveDownloadHandler)

	router.POST("/downloads", FileDownloadCreateHandler)
	router.GET("/downloads/:secret/:file-id", FileDownloadHandler)

	router.POST("/:file-id/relationships/referenced_by", AddReferencedHandler)
	router.DELETE("/:file-id/relationships/referenced_by", RemoveReferencedHandler)

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
	case vfs.ErrParentInTrash:
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
	case vfs.ErrFileInTrash, vfs.ErrNonAbsolutePath,
		vfs.ErrDirNotEmpty:
		return jsonapi.BadRequest(err)
	case vfs.ErrFileTooBig:
		return jsonapi.NewError(http.StatusRequestEntityTooLarge, err)
	}
	return err
}

// FileDocFromReq creates a FileDoc from an incoming request.
func FileDocFromReq(c echo.Context, name, dirID string, tags []string) (*vfs.FileDoc, error) {
	header := c.Request().Header

	size, err := parseContentLength(header.Get("Content-Length"))
	if err != nil {
		err = jsonapi.InvalidParameter("Content-Length", err)
		return nil, err
	}

	var md5Sum []byte
	if md5Str := header.Get("Content-MD5"); md5Str != "" {
		md5Sum, err = parseMD5Hash(md5Str)
	}
	if err != nil {
		err = jsonapi.InvalidParameter("Content-MD5", err)
		return nil, err
	}

	cdate := time.Now()
	if date := header.Get("Date"); date != "" {
		if t, err := time.Parse(time.RFC1123, date); err == nil {
			cdate = t
		}
	}

	var mime, class string
	contentType := header.Get("Content-Type")
	if contentType == "" {
		mime, class = vfs.ExtractMimeAndClassFromFilename(name)
	} else {
		mime, class = vfs.ExtractMimeAndClass(contentType)
	}

	executable := c.QueryParam("Executable") == "true"
	trashed := false
	return vfs.NewFileDoc(
		name,
		dirID,
		size,
		md5Sum,
		mime,
		class,
		cdate,
		executable,
		trashed,
		tags,
	)
}

// CheckIfMatch checks if the revision provided matches the revision number
// given in the request, in the header and/or the query.
func CheckIfMatch(c echo.Context, rev string) error {
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
		return jsonapi.PreconditionFailed("If-Match", fmt.Errorf("Revision does not match"))
	}
	return nil
}

func checkPerm(c echo.Context, v pkgperm.Verb, d *vfs.DirDoc, f *vfs.FileDoc) error {
	if d != nil {
		return permissions.AllowVFS(c, v, d)
	}

	return permissions.AllowVFS(c, v, f)
}

func parseMD5Hash(md5B64 string) ([]byte, error) {
	// Encoded md5 hash in base64 should at least have 22 caracters in
	// base64: 16*3/4 = 21+1/3
	//
	// The padding may add up to 2 characters (non useful). If we are
	// out of these boundaries we know we don't have a good hash and we
	// can bail immediately.
	if len(md5B64) < 22 || len(md5B64) > 24 {
		return nil, fmt.Errorf("Given Content-MD5 is invalid")
	}

	md5Sum, err := base64.StdEncoding.DecodeString(md5B64)
	if err != nil || len(md5Sum) != 16 {
		return nil, fmt.Errorf("Given Content-MD5 is invalid")
	}

	return md5Sum, nil
}

func parseContentLength(contentLength string) (int64, error) {
	if contentLength == "" {
		return -1, nil
	}

	size, err := strconv.ParseInt(contentLength, 10, 64)
	if err != nil {
		err = fmt.Errorf("Invalid content length")
	}
	return size, err
}
