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
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/pkg/logger"
	statikFS "github.com/cozy/cozy-stack/pkg/statik/fs"
	"github.com/cozy/cozy-stack/pkg/utils"
	web_utils "github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/worker/thumbnail"
	"github.com/cozy/echo"
)

type docPatch struct {
	docID   string
	docPath string

	Trash  bool `json:"move_to_trash,omitempty"`
	Delete bool `json:"permanent_delete,omitempty"`
	vfs.DocPatch
}

// TagSeparator is the character separating tags
const TagSeparator = ","

// ErrDocTypeInvalid is used when the document type sent is not
// recognized
var ErrDocTypeInvalid = errors.New("Invalid document type")

// CreationHandler handle all POST requests on /files/:file-id
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
		return WrapVfsError(err)
	}

	return jsonapi.Data(c, http.StatusCreated, doc, nil)
}

func createFileHandler(c echo.Context, fs vfs.VFS) (f *file, err error) {
	tags := strings.Split(c.QueryParam("Tags"), TagSeparator)

	dirID := c.Param("file-id")
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

	instance := middlewares.GetInstance(c)
	defer func() {
		if cerr := file.Close(); cerr != nil && (err == nil || err == io.ErrUnexpectedEOF) {
			err = cerr
			instance.Logger().WithField("nspace", "files").
				Warnf("Error on uploading file (close): %s", err)
		}
	}()

	n, err := io.Copy(file, c.Request().Body)
	if err != nil {
		cl := c.Request().ContentLength
		instance.Logger().WithField("nspace", "files").
			Warnf("Error on uploading file (copy): %s (%d bytes written - expected %d)", err, n, cl)
		return
	}
	f = newFile(doc, instance)
	return
}

func createDirHandler(c echo.Context, fs vfs.VFS) (*dir, error) {
	path := c.QueryParam("Path")
	tags := utils.SplitTrimString(c.QueryParam("Tags"), TagSeparator)

	var doc *vfs.DirDoc
	var err error
	if path != "" {
		if c.QueryParam("Recursive") == "true" {
			doc, err = vfs.MkdirAll(fs, path)
		} else {
			doc, err = vfs.Mkdir(fs, path, tags)
		}
		if err != nil {
			return nil, err
		}
		return newDir(doc), nil
	}

	dirID := c.Param("file-id")
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

	fileID := c.Param("file-id")
	if fileID == "" {
		fileID = c.Param("docid") // Used by sharings.updateDocument
	}

	olddoc, err = instance.VFS().FileByID(fileID)
	if err != nil {
		return WrapVfsError(err)
	}

	newdoc, err = FileDocFromReq(
		c,
		olddoc.DocName,
		olddoc.DirID,
		olddoc.Tags,
	)
	if err != nil {
		return WrapVfsError(err)
	}

	newdoc.ReferencedBy = olddoc.ReferencedBy

	if err = CheckIfMatch(c, olddoc.Rev()); err != nil {
		return WrapVfsError(err)
	}

	err = checkPerm(c, permission.PUT, nil, olddoc)
	if err != nil {
		return
	}

	newdoc.SetID(olddoc.ID()) // The ID can be useful to check permissions
	err = checkPerm(c, permission.PUT, nil, newdoc)
	if err != nil {
		return
	}

	file, err := instance.VFS().CreateFile(newdoc, olddoc)
	if err != nil {
		return WrapVfsError(err)
	}

	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
		if err != nil {
			err = WrapVfsError(err)
			return
		}
		err = fileData(c, http.StatusOK, newdoc, nil)
	}()

	_, err = io.Copy(file, c.Request().Body)
	return
}

// ModifyMetadataByIDHandler handles PATCH requests on /files/:file-id
//
// It can be used to modify the file or directory metadata, as well as
// moving and renaming it in the filesystem.
func ModifyMetadataByIDHandler(c echo.Context) error {
	patch, err := getPatch(c, c.Param("file-id"), "")
	if err != nil {
		return WrapVfsError(err)
	}
	i := middlewares.GetInstance(c)
	if err = applyPatch(c, i.VFS(), patch); err != nil {
		return WrapVfsError(err)
	}
	return nil
}

// ModifyMetadataByIDInBatchHandler handles PATCH requests on /files/.
//
// It can be used to modify many files or directories metadata, as well as
// moving and renaming it in the filesystem, in batch.
func ModifyMetadataByIDInBatchHandler(c echo.Context) error {
	patches, err := getPatches(c)
	if err != nil {
		return WrapVfsError(err)
	}
	i := middlewares.GetInstance(c)
	patchErrors, err := applyPatches(c, i.VFS(), patches)
	if err != nil {
		return err
	}
	if len(patchErrors) > 0 {
		return jsonapi.DataErrorList(c, patchErrors...)
	}
	return c.NoContent(http.StatusNoContent)
}

// ModifyMetadataByPathHandler handles PATCH requests on /files/:file-id
//
// It can be used to modify the file or directory metadata, as well as
// moving and renaming it in the filesystem.
func ModifyMetadataByPathHandler(c echo.Context) error {
	patch, err := getPatch(c, "", c.QueryParam("Path"))
	if err != nil {
		return WrapVfsError(err)
	}
	i := middlewares.GetInstance(c)
	if err = applyPatch(c, i.VFS(), patch); err != nil {
		return WrapVfsError(err)
	}
	return nil
}

func getPatch(c echo.Context, docID, docPath string) (*docPatch, error) {
	var patch docPatch
	obj, err := jsonapi.Bind(c.Request().Body, &patch)
	if err != nil {
		return nil, jsonapi.BadJSON()
	}
	patch.docID = docID
	patch.docPath = docPath
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

func getPatches(c echo.Context) ([]*docPatch, error) {
	req := c.Request()
	objs, err := jsonapi.BindCompound(req.Body)
	if err != nil {
		return nil, jsonapi.BadJSON()
	}
	patches := make([]*docPatch, len(objs))
	for i, obj := range objs {
		var patch docPatch
		if obj.Attributes == nil {
			return nil, jsonapi.BadJSON()
		}
		if err = json.Unmarshal(*obj.Attributes, &patch); err != nil {
			return nil, err
		}
		patch.docID = obj.ID
		patch.docPath = ""
		patch.RestorePath = nil
		if rel, ok := obj.GetRelationship("parent"); ok {
			rid, ok := rel.ResourceIdentifier()
			if !ok {
				return nil, jsonapi.BadJSON()
			}
			patch.DirID = &rid.ID
		}
		patches[i] = &patch
	}
	return patches, nil
}

func applyPatch(c echo.Context, fs vfs.VFS, patch *docPatch) (err error) {
	var file *vfs.FileDoc
	var dir *vfs.DirDoc
	if patch.docID != "" {
		dir, file, err = fs.DirOrFileByID(patch.docID)
	} else {
		dir, file, err = fs.DirOrFileByPath(patch.docPath)
	}
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
		return err
	}

	if err = checkPerm(c, permission.PATCH, dir, file); err != nil {
		return err
	}

	if patch.Delete {
		if dir != nil {
			err = fs.DestroyDirAndContent(dir)
		} else {
			err = fs.DestroyFile(file)
		}
	} else if patch.Trash {
		if dir != nil {
			dir, err = vfs.TrashDir(fs, dir)
		} else {
			file, err = vfs.TrashFile(fs, file)
		}
	} else {
		if dir != nil {
			dir, err = vfs.ModifyDirMetadata(fs, dir, &patch.DocPatch)
		} else {
			file, err = vfs.ModifyFileMetadata(fs, file, &patch.DocPatch)
		}
	}
	if err != nil {
		return err
	}

	if dir != nil {
		return dirData(c, http.StatusOK, dir)
	}
	return fileData(c, http.StatusOK, file, nil)
}

func applyPatches(c echo.Context, fs vfs.VFS, patches []*docPatch) (errors []*jsonapi.Error, err error) {
	for _, patch := range patches {
		dir, file, errf := fs.DirOrFileByID(patch.docID)
		if errf != nil {
			jsonapiError := wrapVfsErrorJSONAPI(errf)
			jsonapiError.Source.Parameter = "_id"
			jsonapiError.Source.Pointer = patch.docID
			errors = append(errors, jsonapiError)
			continue
		}
		if err = checkPerm(c, permission.PATCH, dir, file); err != nil {
			return
		}
		var errp error
		if patch.Delete {
			if dir != nil {
				errp = fs.DestroyDirAndContent(dir)
			} else if file != nil {
				errp = fs.DestroyFile(file)
			}
		} else if patch.Trash {
			if dir != nil {
				_, errp = vfs.TrashDir(fs, dir)
			} else if file != nil {
				_, errp = vfs.TrashFile(fs, file)
			}
		} else if dir != nil {
			_, errp = vfs.ModifyDirMetadata(fs, dir, &patch.DocPatch)
		} else if file != nil {
			_, errp = vfs.ModifyFileMetadata(fs, file, &patch.DocPatch)
		}
		if errp != nil {
			jsonapiError := wrapVfsErrorJSONAPI(errp)
			jsonapiError.Source.Parameter = "_id"
			jsonapiError.Source.Pointer = patch.docID
			errors = append(errors, jsonapiError)
		}
	}

	return
}

// ReadMetadataFromIDHandler handles all GET requests on /files/:file-
// id aiming at getting file metadata from its id.
func ReadMetadataFromIDHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	perm, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}

	fileID := c.Param("file-id")

	dir, file, err := instance.VFS().DirOrFileByID(fileID)
	if err != nil {
		return WrapVfsError(err)
	}

	if err := checkPerm(c, permission.GET, dir, file); err != nil {
		return err
	}

	// Limiting the number of public share link consultations
	if perm.Type == permission.TypeShareByLink {
		err = limits.CheckRateLimitKey(fileID, limits.SharingPublicLinkType)
		if err != nil {
			return err
		}
	}

	if dir != nil {
		return dirData(c, http.StatusOK, dir)
	}
	return fileData(c, http.StatusOK, file, nil)
}

// GetChildrenHandler returns a list of children of a folder
func GetChildrenHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	dir, file, err := instance.VFS().DirOrFileByID(fileID)
	if err != nil {
		return WrapVfsError(err)
	}

	if file != nil {
		return jsonapi.Errorf(http.StatusBadRequest, "cant read children of file %v", fileID)
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
		return WrapVfsError(err)
	}

	if err := checkPerm(c, permission.GET, dir, file); err != nil {
		return err
	}

	if dir != nil {
		return dirData(c, http.StatusOK, dir)
	}
	return fileData(c, http.StatusOK, file, nil)
}

// ReadFileContentFromIDHandler handles all GET requests on /files/:file-id
// aiming at downloading a file given its ID. It serves the file in inline
// mode.
func ReadFileContentFromIDHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	doc, err := instance.VFS().FileByID(c.Param("file-id"))
	if err != nil {
		return WrapVfsError(err)
	}

	err = checkPerm(c, permission.GET, nil, doc)
	if err != nil {
		return err
	}

	disposition := "inline"
	if c.QueryParam("Dl") == "1" {
		disposition = "attachment"
	}
	err = vfs.ServeFileContent(instance.VFS(), doc, disposition, c.Request(), c.Response())
	if err != nil {
		return WrapVfsError(err)
	}

	return nil
}

// HeadDirOrFile handles HEAD requests on directory or file to check their
// existence
func HeadDirOrFile(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	dir, file, err := instance.VFS().DirOrFileByID(c.Param("file-id"))
	if err != nil {
		return WrapVfsError(err)
	}

	if dir != nil {
		err = checkPerm(c, permission.GET, dir, nil)
	} else {
		err = checkPerm(c, permission.GET, nil, file)
	}
	if err != nil {
		return err
	}

	return nil
}

// ThumbnailHandler serves thumbnails of the images/photos
func ThumbnailHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	secret := c.Param("secret")
	path, err := vfs.GetStore().GetFile(instance, secret)
	if err != nil {
		return WrapVfsError(err)
	}
	if path == "" {
		return jsonapi.NewError(http.StatusBadRequest, "Wrong download token")
	}

	doc, err := instance.VFS().FileByID(c.Param("file-id"))
	if err != nil {
		return WrapVfsError(err)
	}

	expected, err := doc.Path(instance.VFS())
	if err != nil {
		return WrapVfsError(err)
	}
	if expected != path {
		return jsonapi.NewError(http.StatusBadRequest, "Wrong download token")
	}

	fs := instance.ThumbsFS()
	format := c.Param("format")
	err = fs.ServeThumbContent(c.Response(), c.Request(), doc, format)
	if err != nil {
		return serveThumbnailPlaceholder(c.Response(), c.Request(), doc, format)
	}
	return nil
}

func serveThumbnailPlaceholder(res http.ResponseWriter, req *http.Request, doc *vfs.FileDoc, format string) error {
	if !utils.IsInArray(format, thumbnail.FormatsNames) {
		return echo.NewHTTPError(http.StatusNotFound, "Format does not exist")
	}
	f, ok := statikFS.Get("/placeholders/thumbnail-"+format+".png", "")
	if !ok {
		return os.ErrNotExist
	}
	etag := f.Etag
	if web_utils.CheckPreconditions(res, req, etag) {
		return nil
	}
	res.Header().Set("Etag", etag)
	_, err := io.Copy(res, f.Reader())
	return err
}

func sendFileFromPath(c echo.Context, path string, checkPermission bool) error {
	instance := middlewares.GetInstance(c)

	doc, err := instance.VFS().FileByPath(path)
	if err != nil {
		return WrapVfsError(err)
	}

	if checkPermission {
		err = middlewares.Allow(c, permission.GET, doc)
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
			middlewares.AppendCSPRule(c, "frame-ancestors", "*")
		}
	}
	err = vfs.ServeFileContent(instance.VFS(), doc, disposition, c.Request(), c.Response())
	if err != nil {
		return WrapVfsError(err)
	}

	return nil
}

// ReadFileContentFromPathHandler handles all GET request on /files/download
// aiming at downloading a file given its path. It serves the file in in
// attachment mode.
func ReadFileContentFromPathHandler(c echo.Context) error {
	return sendFileFromPath(c, c.QueryParam("Path"), true)
}

// ArchiveDownloadCreateHandler handles requests to /files/archive and stores the
// paremeters with a secret to be used in download handler below.s
func ArchiveDownloadCreateHandler(c echo.Context) error {
	archive := &vfs.Archive{}
	if _, err := jsonapi.Bind(c.Request().Body, archive); err != nil {
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
		return WrapVfsError(err)
	}

	for _, e := range entries {
		err = checkPerm(c, permission.GET, e.Dir, e.File)
		if err != nil {
			return err
		}
	}

	// if accept header is application/zip, send the archive immediately
	if c.Request().Header.Get("Accept") == "application/zip" {
		return archive.Serve(instance.VFS(), c.Response())
	}

	secret, err := vfs.GetStore().AddArchive(instance, archive)
	if err != nil {
		return WrapVfsError(err)
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
	var path string

	if path = c.QueryParam("Path"); path != "" {
		if doc, err = instance.VFS().FileByPath(path); err != nil {
			return WrapVfsError(err)
		}
	} else if id := c.QueryParam("Id"); id != "" {
		if doc, err = instance.VFS().FileByID(id); err != nil {
			return WrapVfsError(err)
		}
		if path, err = doc.Path(instance.VFS()); err != nil {
			return WrapVfsError(err)
		}
	}

	err = checkPerm(c, "GET", nil, doc)
	if err != nil {
		return err
	}

	secret, err := vfs.GetStore().AddFile(instance, path)
	if err != nil {
		return WrapVfsError(err)
	}

	links := &jsonapi.LinksList{
		Related: "/files/downloads/" + secret + "/" + doc.DocName,
	}

	return fileData(c, http.StatusOK, doc, links)
}

// ArchiveDownloadHandler handles requests to /files/archive/:secret/whatever.zip
// and creates on the fly zip archive from the parameters linked to secret.
func ArchiveDownloadHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	secret := c.Param("secret")
	archive, err := vfs.GetStore().GetArchive(instance, secret)
	if err != nil {
		return WrapVfsError(err)
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
	path, err := vfs.GetStore().GetFile(instance, secret)
	if err != nil {
		return WrapVfsError(err)
	}
	if path == "" {
		return jsonapi.NewError(http.StatusBadRequest, "Wrong download token")
	}
	return sendFileFromPath(c, path, false)
}

// TrashHandler handles all DELETE requests on /files/:file-id and
// moves the file or directory with the specified file-id to the
// trash.
func TrashHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")
	dir, file, err := instance.VFS().DirOrFileByID(fileID)
	if err != nil {
		return WrapVfsError(err)
	}

	err = checkPerm(c, permission.PUT, dir, file)
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
		return WrapVfsError(err)
	}

	if dir != nil {
		doc, errt := vfs.TrashDir(instance.VFS(), dir)
		if errt != nil {
			return WrapVfsError(errt)
		}
		return dirData(c, http.StatusOK, doc)
	}

	doc, errt := vfs.TrashFile(instance.VFS(), file)
	if errt != nil {
		return WrapVfsError(errt)
	}
	return fileData(c, http.StatusOK, doc, nil)
}

// ReadTrashFilesHandler handle GET requests on /files/trash and return the
// list of trashed files and directories
func ReadTrashFilesHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	trash, err := instance.VFS().DirByID(consts.TrashDirID)
	if err != nil {
		return WrapVfsError(err)
	}

	err = checkPerm(c, permission.GET, trash, nil)
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
		return WrapVfsError(err)
	}

	err = checkPerm(c, permission.PUT, dir, file)
	if err != nil {
		return err
	}

	if dir != nil {
		doc, errt := vfs.RestoreDir(instance.VFS(), dir)
		if errt != nil {
			return WrapVfsError(errt)
		}
		return dirData(c, http.StatusOK, doc)
	}

	doc, errt := vfs.RestoreFile(instance.VFS(), file)
	if errt != nil {
		return WrapVfsError(errt)
	}
	return fileData(c, http.StatusOK, doc, nil)
}

// ClearTrashHandler handles DELETE request to clear the trash
func ClearTrashHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	trash, err := instance.VFS().DirByID(consts.TrashDirID)
	if err != nil {
		return WrapVfsError(err)
	}

	err = checkPerm(c, permission.DELETE, trash, nil)
	if err != nil {
		return err
	}

	err = instance.VFS().DestroyDirContent(trash)
	if err != nil {
		return WrapVfsError(err)
	}

	return c.NoContent(204)
}

// DestroyFileHandler handles DELETE request to clear one element from the trash
func DestroyFileHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	dir, file, err := instance.VFS().DirOrFileByID(fileID)
	if err != nil {
		return WrapVfsError(err)
	}

	err = checkPerm(c, permission.DELETE, dir, file)
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
		return WrapVfsError(err)
	}

	if dir != nil {
		err = instance.VFS().DestroyDirAndContent(dir)
	} else {
		err = instance.VFS().DestroyFile(file)
	}
	if err != nil {
		return WrapVfsError(err)
	}

	return c.NoContent(204)
}

// FindFilesMango is the route POST /files/_find
// used to retrieve files and their metadata from a mango query.
func FindFilesMango(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	var findRequest map[string]interface{}

	if err := json.NewDecoder(c.Request().Body).Decode(&findRequest); err != nil {
		return jsonapi.Errorf(http.StatusBadRequest, "%s", err)
	}

	if err := middlewares.AllowWholeType(c, permission.GET, consts.Files); err != nil {
		return err
	}

	// drop the fields, they can cause issues if not properly manipulated
	// TODO : optimization potential, necessary fields so far are class & type
	delete(findRequest, "fields")

	limit, hasLimit := findRequest["limit"].(float64)
	if !hasLimit || limit > consts.MaxItemsPerPageForMango {
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
			out[i] = newFile(f, instance)
		}
	}

	return jsonapi.DataListWithTotal(c, http.StatusOK, total, out, nil)

}

func fsckHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	cacheStorage := config.GetConfig().CacheStorage

	if err := middlewares.AllowWholeType(c, permission.GET, consts.Files); err != nil {
		return err
	}

	noCache, _ := strconv.ParseBool(c.QueryParam("NoCache"))
	key := "fsck:" + instance.DBPrefix()
	if !noCache {
		if r, ok := cacheStorage.GetCompressed(key); ok {
			return c.Stream(http.StatusOK, echo.MIMEApplicationJSON, r)
		}
	}

	logs := make([]*vfs.FsckLog, 0)
	err := instance.VFS().Fsck(func(log *vfs.FsckLog) {
		switch log.Type {
		case vfs.ContentMismatch:
			logs = append(logs, log)
		}
	})
	if err != nil {
		return err
	}

	logsData, err := json.Marshal(logs)
	if err != nil {
		return err
	}

	if !noCache {
		expiration := utils.DurationFuzzing(3*30*24*time.Hour, 0.10)
		cacheStorage.SetCompressed(key, logsData, expiration)
	}

	return c.JSONBlob(http.StatusOK, logsData)
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
	router.PATCH("/", ModifyMetadataByIDInBatchHandler)

	router.POST("/", CreationHandler)
	router.POST("/:file-id", CreationHandler)
	router.PUT("/:file-id", OverwriteFileContentHandler)

	router.GET("/:file-id/thumbnails/:secret/:format", ThumbnailHandler)

	router.POST("/archive", ArchiveDownloadCreateHandler)
	router.GET("/archive/:secret/:fake-name", ArchiveDownloadHandler)

	router.POST("/downloads", FileDownloadCreateHandler)
	router.GET("/downloads/:secret/:fake-name", FileDownloadHandler)

	router.POST("/:file-id/relationships/referenced_by", AddReferencedHandler)
	router.DELETE("/:file-id/relationships/referenced_by", RemoveReferencedHandler)

	router.GET("/trash", ReadTrashFilesHandler)
	router.DELETE("/trash", ClearTrashHandler)

	router.POST("/trash/:file-id", RestoreTrashFileHandler)
	router.DELETE("/trash/:file-id", DestroyFileHandler)

	router.DELETE("/:file-id", TrashHandler)
	router.GET("/fsck", fsckHandler)
}

// WrapVfsError returns a formatted error from a golang error emitted by the vfs
func WrapVfsError(err error) error {
	if errj := wrapVfsError(err); errj != nil {
		return errj
	}
	return err
}

func wrapVfsErrorJSONAPI(err error) *jsonapi.Error {
	if errj := wrapVfsError(err); errj != nil {
		return errj
	}
	return jsonapi.InternalServerError(err)
}

func wrapVfsError(err error) *jsonapi.Error {
	switch err {
	case ErrDocTypeInvalid:
		return jsonapi.InvalidAttribute("type", err)
	case os.ErrNotExist:
		return jsonapi.NotFound(err)
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
		return jsonapi.Errorf(http.StatusRequestEntityTooLarge, "%s", err)
	}
	return nil
}

// FileDocFromReq creates a FileDoc from an incoming request.
func FileDocFromReq(c echo.Context, name, dirID string, tags []string) (*vfs.FileDoc, error) {
	header := c.Request().Header
	size := c.Request().ContentLength

	var err error
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
		// Some browsers may use Mime-Type sniffing and they may sent an
		// inaccurate Content-Type.
		ext := strings.ToLower(path.Ext(name))
		if contentType == "application/octet-stream" {
			switch ext {
			case ".heif":
				contentType = "image/heif"
			case ".heic":
				contentType = "image/heic"
			}
		}
		if contentType == "text/xml" && ext == "svg" {
			contentType = "image/svg+xml"
		}
		mime, class = vfs.ExtractMimeAndClass(contentType)
	}

	executable := c.QueryParam("Executable") == "true"
	trashed := false
	doc, err := vfs.NewFileDoc(
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
	if meta := c.QueryParam("Metadata"); err == nil && meta != "" {
		if err := json.Unmarshal([]byte(meta), &doc.Metadata); err != nil {
			logger.WithNamespace("files").Debugf("Invalid Metadata at upload: %s", err)
		}
	}
	return doc, err
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
	return checkIfMatch(rev, wantedRev)
}

func checkIfMatch(rev, wantedRev string) error {
	if wantedRev != "" && rev != wantedRev {
		return jsonapi.PreconditionFailed("If-Match", fmt.Errorf("Revision does not match"))
	}
	return nil
}

func checkPerm(c echo.Context, v permission.Verb, d *vfs.DirDoc, f *vfs.FileDoc) error {
	if d != nil {
		return middlewares.AllowVFS(c, v, d)
	}
	return middlewares.AllowVFS(c, v, f)
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
