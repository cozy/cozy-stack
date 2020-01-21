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

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/assets/statik"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/metadata"
	"github.com/cozy/cozy-stack/pkg/utils"
	web_utils "github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/worker/thumbnail"
	"github.com/labstack/echo/v4"
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
	dirID := c.Param("file-id")
	name := c.QueryParam("Name")
	var doc *vfs.FileDoc
	doc, err = FileDocFromReq(c, name, dirID)
	if err != nil {
		return
	}

	if created := c.QueryParam("CreatedAt"); created != "" {
		if at, err2 := time.Parse(time.RFC3339, created); err2 == nil {
			doc.CreatedAt = at
		}
	}
	doc.CozyMetadata, _ = cozyMetadataFromClaims(c, true)

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
	f = NewFile(doc, instance)
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
	if created := c.QueryParam("CreatedAt"); created != "" {
		if at, err2 := time.Parse(time.RFC3339, created); err2 == nil {
			doc.CreatedAt = at
		}
	}

	doc.CozyMetadata, _ = cozyMetadataFromClaims(c, false)

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

	newdoc, err = FileDocFromReq(c, olddoc.DocName, olddoc.DirID)
	if err != nil {
		return WrapVfsError(err)
	}

	newdoc.ReferencedBy = olddoc.ReferencedBy

	if err = CheckIfMatch(c, olddoc.Rev()); err != nil {
		return WrapVfsError(err)
	}

	if olddoc.CozyMetadata != nil {
		newdoc.CozyMetadata = olddoc.CozyMetadata.Clone()
	}
	updateFileCozyMetadata(c, newdoc, true)

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
		err = FileData(c, http.StatusOK, newdoc, true, nil)
	}()

	_, err = io.Copy(file, c.Request().Body)
	return
}

// UploadMetadataHandler accepts a metadata objet and persist it, so that it
// can be used in a future file upload.
func UploadMetadataHandler(c echo.Context) error {
	if err := checkPerm(c, permission.POST, nil, &vfs.FileDoc{}); err != nil {
		return err
	}

	meta := &vfs.Metadata{}
	if _, err := jsonapi.Bind(c.Request().Body, meta); err != nil {
		return err
	}

	instance := middlewares.GetInstance(c)
	secret, err := vfs.GetStore().AddMetadata(instance, meta)
	if err != nil {
		return WrapVfsError(err)
	}

	m := apiMetadata{
		Metadata: meta,
		secret:   secret,
	}
	return jsonapi.Data(c, http.StatusCreated, &m, nil)
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

// ModifyMetadataByPathHandler handles PATCH requests on /files/metadata
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

// ModifyFileVersionMetadata handles PATCH requests on /files/:file-id/:version-id
//
// It can be used to modify tags on an old version of a file.
func ModifyFileVersionMetadata(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	fileID := c.Param("file-id")
	_, file, err := inst.VFS().DirOrFileByID(fileID)
	if err != nil {
		return WrapVfsError(err)
	}
	if file == nil {
		return WrapVfsError(vfs.ErrConflict)
	}
	if err = checkPerm(c, permission.PATCH, nil, file); err != nil {
		return WrapVfsError(err)
	}
	docID := fileID + "/" + c.Param("version-id")
	version, err := vfs.FindVersion(inst, docID)
	if err != nil {
		return WrapVfsError(err)
	}
	var patch vfs.DocPatch
	if _, err = jsonapi.Bind(c.Request().Body, &patch); err != nil || patch.Tags == nil {
		return jsonapi.BadJSON()
	}
	version.Tags = *patch.Tags
	version.CozyMetadata.UpdatedAt = time.Now()
	if err = couchdb.UpdateDoc(inst, version); err != nil {
		return WrapVfsError(err)
	}
	return jsonapi.Data(c, http.StatusOK, version, nil)
}

// DeleteFileVersionMetadata handles DELETE requests on /files/:file-id/:version-id
//
// It can be used to delete an old version of a file.
func DeleteFileVersionMetadata(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	fs := inst.VFS()
	fileID := c.Param("file-id")
	_, file, err := fs.DirOrFileByID(fileID)
	if err != nil {
		return WrapVfsError(err)
	}
	if file == nil {
		return WrapVfsError(vfs.ErrConflict)
	}
	if err = checkPerm(c, permission.DELETE, nil, file); err != nil {
		return WrapVfsError(err)
	}
	docID := fileID + "/" + c.Param("version-id")
	version, err := vfs.FindVersion(inst, docID)
	if err != nil {
		return WrapVfsError(err)
	}
	if err := fs.CleanOldVersion(fileID, version); err != nil {
		return WrapVfsError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

// CopyVersionHandler handles POST requests on /files/:file-id/versions.
//
// It can be used to create a new version of a file, with the same content but
// new metadata.
func CopyVersionHandler(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	fs := inst.VFS()
	fileID := c.Param("file-id")
	olddoc, err := fs.FileByID(fileID)
	if err != nil {
		return WrapVfsError(err)
	}
	if olddoc == nil {
		return WrapVfsError(vfs.ErrConflict)
	}
	if err = checkPerm(c, permission.PUT, nil, olddoc); err != nil {
		return WrapVfsError(err)
	}

	meta := vfs.Metadata{}
	if _, err := jsonapi.Bind(c.Request().Body, &meta); err != nil {
		return err
	}

	newdoc := olddoc.Clone().(*vfs.FileDoc)
	newdoc.Metadata = meta
	newdoc.Tags = utils.SplitTrimString(c.QueryParam("Tags"), TagSeparator)
	updateFileCozyMetadata(c, newdoc, true)

	content, err := fs.OpenFile(olddoc)
	if err != nil {
		return WrapVfsError(err)
	}
	defer content.Close()

	file, err := fs.CreateFile(newdoc, olddoc)
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
		err = FileData(c, http.StatusOK, newdoc, true, nil)
	}()

	_, err = io.Copy(file, content)
	return err
}

// ClearOldVersions is the handler for DELETE /files/versions.
// It deletes all the old versions of all files to make space for new files.
func ClearOldVersions(c echo.Context) error {
	if err := middlewares.AllowWholeType(c, permission.DELETE, consts.Files); err != nil {
		return err
	}

	fs := middlewares.GetInstance(c).VFS()
	if err := fs.ClearOldVersions(); err != nil {
		return WrapVfsError(err)
	}

	return c.NoContent(204)
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
			inst := middlewares.GetInstance(c)
			err = fs.DestroyDirAndContent(dir, pushTrashJob(inst))
		} else {
			err = fs.DestroyFile(file)
		}
	} else if patch.Trash {
		if dir != nil {
			updateDirCozyMetadata(c, dir)
			dir, err = vfs.TrashDir(fs, dir)
		} else {
			updateFileCozyMetadata(c, file, false)
			file, err = vfs.TrashFile(fs, file)
		}
	} else {
		if dir != nil {
			updateDirCozyMetadata(c, dir)
			dir, err = vfs.ModifyDirMetadata(fs, dir, &patch.DocPatch)
		} else {
			updateFileCozyMetadata(c, file, false)
			file, err = vfs.ModifyFileMetadata(fs, file, &patch.DocPatch)
		}
	}
	if err != nil {
		return err
	}

	if dir != nil {
		return dirData(c, http.StatusOK, dir)
	}
	return FileData(c, http.StatusOK, file, false, nil)
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
				inst := middlewares.GetInstance(c)
				errp = fs.DestroyDirAndContent(dir, pushTrashJob(inst))
			} else if file != nil {
				errp = fs.DestroyFile(file)
			}
		} else if patch.Trash {
			if dir != nil {
				updateDirCozyMetadata(c, dir)
				_, errp = vfs.TrashDir(fs, dir)
			} else if file != nil {
				updateFileCozyMetadata(c, file, false)
				_, errp = vfs.TrashFile(fs, file)
			}
		} else if dir != nil {
			updateDirCozyMetadata(c, dir)
			_, errp = vfs.ModifyDirMetadata(fs, dir, &patch.DocPatch)
		} else if file != nil {
			updateFileCozyMetadata(c, file, false)
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
		if limits.IsLimitReachedOrExceeded(err) {
			return err
		}
	}

	if dir != nil {
		return dirData(c, http.StatusOK, dir)
	}
	return FileData(c, http.StatusOK, file, true, nil)
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
	return FileData(c, http.StatusOK, file, true, nil)
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
	err = vfs.ServeFileContent(instance.VFS(), doc, nil, "", disposition, c.Request(), c.Response())
	if err != nil {
		return WrapVfsError(err)
	}

	return nil
}

// ReadFileContentFromVersion handles the download of an old version of the
// file content.
func ReadFileContentFromVersion(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	doc, err := instance.VFS().FileByID(c.Param("file-id"))
	if err != nil {
		return WrapVfsError(err)
	}

	err = checkPerm(c, permission.GET, nil, doc)
	if err != nil {
		return err
	}

	version, err := vfs.FindVersion(instance, doc.DocID+"/"+c.Param("version-id"))
	if err != nil {
		return WrapVfsError(err)
	}

	disposition := "inline"
	if c.QueryParam("Dl") == "1" {
		disposition = "attachment"
	}
	err = vfs.ServeFileContent(instance.VFS(), doc, version, "", disposition, c.Request(), c.Response())
	if err != nil {
		return WrapVfsError(err)
	}

	return nil
}

// RevertFileVersion restores an old version of the file content.
func RevertFileVersion(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	doc, err := inst.VFS().FileByID(c.Param("file-id"))
	if err != nil {
		return WrapVfsError(err)
	}

	if err = checkPerm(c, permission.POST, nil, doc); err != nil {
		return err
	}

	version, err := vfs.FindVersion(inst, doc.DocID+"/"+c.Param("version-id"))
	if err != nil {
		return WrapVfsError(err)
	}

	if err = inst.VFS().RevertFileVersion(doc, version); err != nil {
		return WrapVfsError(err)
	}

	return FileData(c, http.StatusOK, doc, true, nil)
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
	fileID, err := vfs.GetStore().GetThumb(instance, secret)
	if err != nil {
		return WrapVfsError(err)
	}
	if fileID == "" || c.Param("file-id") != fileID {
		return jsonapi.NewError(http.StatusBadRequest, "Wrong download token")
	}

	doc, err := instance.VFS().FileByID(fileID)
	if err != nil {
		return WrapVfsError(err)
	}

	fs := lifecycle.ThumbsFS(instance)
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
	f := statik.GetAsset("/placeholders/thumbnail-" + format + ".png")
	if f == nil {
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
		addCSPRuleForDirectLink(c, doc.Class, doc.Mime)
	}
	err = vfs.ServeFileContent(instance.VFS(), doc, nil, "", disposition, c.Request(), c.Response())
	if err != nil {
		return WrapVfsError(err)
	}

	return nil
}

func addCSPRuleForDirectLink(c echo.Context, class, mime string) {
	// Allow some files to be displayed by the browser in the client-side apps
	if mime == "text/plain" || class == "image" || class == "audio" || class == "video" || mime == "application/pdf" {
		middlewares.AppendCSPRule(c, "frame-ancestors", "*")
	}
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
	var versionID string

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
	} else if versionID = c.QueryParam("VersionId"); versionID != "" {
		docID := strings.Split(versionID, "/")[0]
		if doc, err = instance.VFS().FileByID(docID); err != nil {
			return WrapVfsError(err)
		}
	}

	err = checkPerm(c, "GET", nil, doc)
	if err != nil {
		return err
	}

	var secret string
	if versionID == "" {
		secret, err = vfs.GetStore().AddFile(instance, path)
	} else {
		secret, err = vfs.GetStore().AddVersion(instance, versionID)
		secret = "v-" + secret
	}
	if err != nil {
		return WrapVfsError(err)
	}

	filename := c.QueryParam("Filename")
	if filename == "" {
		filename = doc.DocName
	}
	links := &jsonapi.LinksList{
		Related: "/files/downloads/" + secret + "/" + filename,
	}

	return FileData(c, http.StatusOK, doc, false, links)
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
	if err := archive.Serve(instance.VFS(), c.Response()); err != nil {
		return WrapVfsError(err)
	}
	return nil
}

// FileDownloadHandler send a file that have previously be defined
// through FileDownloadCreateHandler
func FileDownloadHandler(c echo.Context) error {
	secret := c.Param("secret")
	if strings.HasPrefix(secret, "v-") {
		return versionDownloadHandler(c, strings.TrimPrefix(secret, "v-"))
	}
	instance := middlewares.GetInstance(c)
	path, err := vfs.GetStore().GetFile(instance, secret)
	if err != nil {
		return WrapVfsError(err)
	}
	if path == "" {
		return jsonapi.NewError(http.StatusBadRequest, "Wrong download token")
	}
	return sendFileFromPath(c, path, false)
}

func versionDownloadHandler(c echo.Context, secret string) error {
	instance := middlewares.GetInstance(c)
	versionID, err := vfs.GetStore().GetVersion(instance, secret)
	if err != nil {
		return WrapVfsError(err)
	}
	if versionID == "" {
		return jsonapi.NewError(http.StatusBadRequest, "Wrong download token")
	}

	fileID := strings.Split(versionID, "/")[0]
	doc, err := instance.VFS().FileByID(fileID)
	if err != nil {
		return WrapVfsError(err)
	}
	version, err := vfs.FindVersion(instance, versionID)
	if err != nil {
		return WrapVfsError(err)
	}

	disposition := "inline"
	if c.QueryParam("Dl") == "1" {
		disposition = "attachment"
	} else {
		addCSPRuleForDirectLink(c, doc.Class, doc.Mime)
	}

	filename := c.Param("fake-name")
	err = vfs.ServeFileContent(instance.VFS(), doc, version, filename, disposition, c.Request(), c.Response())
	if err != nil {
		return WrapVfsError(err)
	}
	return nil
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

	err = checkPerm(c, permission.PATCH, dir, file)
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
		updateDirCozyMetadata(c, dir)
		doc, errt := vfs.TrashDir(instance.VFS(), dir)
		if errt != nil {
			return WrapVfsError(errt)
		}
		return dirData(c, http.StatusOK, doc)
	}

	updateFileCozyMetadata(c, file, false)
	doc, errt := vfs.TrashFile(instance.VFS(), file)
	if errt != nil {
		return WrapVfsError(errt)
	}
	return FileData(c, http.StatusOK, doc, false, nil)
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

	err = checkPerm(c, permission.PATCH, dir, file)
	if err != nil {
		return err
	}

	if dir != nil {
		updateDirCozyMetadata(c, dir)
		doc, errt := vfs.RestoreDir(instance.VFS(), dir)
		if errt != nil {
			return WrapVfsError(errt)
		}
		return dirData(c, http.StatusOK, doc)
	}

	updateFileCozyMetadata(c, file, false)
	doc, errt := vfs.RestoreFile(instance.VFS(), file)
	if errt != nil {
		return WrapVfsError(errt)
	}
	return FileData(c, http.StatusOK, doc, false, nil)
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

	err = instance.VFS().DestroyDirContent(trash, pushTrashJob(instance))
	if err != nil {
		return WrapVfsError(err)
	}

	return c.NoContent(204)
}

// DestroyFileHandler handles DELETE request to clear one element from the trash
func DestroyFileHandler(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	dir, file, err := inst.VFS().DirOrFileByID(fileID)
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
		err = inst.VFS().DestroyDirAndContent(dir, pushTrashJob(inst))
	} else {
		err = inst.VFS().DestroyFile(file)
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
	_, err := couchdb.FindDocsRaw(instance, consts.Files, &findRequest, &results)
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
			out[i] = NewFile(f, instance)
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
	err := instance.VFS().CheckFilesConsistency(func(log *vfs.FsckLog) {
		switch log.Type {
		case vfs.ContentMismatch:
			logs = append(logs, log)
		}
	}, false)
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

	router.HEAD("/download/:file-id/:version-id", ReadFileContentFromVersion)
	router.GET("/download/:file-id/:version-id", ReadFileContentFromVersion)
	router.POST("/revert/:file-id/:version-id", RevertFileVersion)
	router.PATCH("/:file-id/:version-id", ModifyFileVersionMetadata)
	router.DELETE("/:file-id/:version-id", DeleteFileVersionMetadata)
	router.POST("/:file-id/versions", CopyVersionHandler)
	router.DELETE("/versions", ClearOldVersions)

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
	router.POST("/upload/metadata", UploadMetadataHandler)

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
	if _, ok := err.(*jsonapi.Error); !ok {
		logger.WithNamespace("files").Warnf("Not wrapped error: %s", err)
	}
	return nil
}

// FileDocFromReq creates a FileDoc from an incoming request.
func FileDocFromReq(c echo.Context, name, dirID string) (*vfs.FileDoc, error) {
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

	tags := strings.Split(c.QueryParam("Tags"), TagSeparator)
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
	if err != nil {
		return nil, err
	}

	// This way to send metadata is deprecated, but is still here to ensure
	// compatibility with existing clients.
	if meta := c.QueryParam("Metadata"); meta != "" {
		if err := json.Unmarshal([]byte(meta), &doc.Metadata); err != nil {
			return nil, err
		}
	}

	if secret := c.QueryParam("MetadataID"); secret != "" {
		instance := middlewares.GetInstance(c)
		meta, err := vfs.GetStore().GetMetadata(instance, secret)
		if err != nil {
			return nil, err
		}
		doc.Metadata = *meta
	}

	return doc, nil
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

func pushTrashJob(inst *instance.Instance) func(vfs.TrashJournal) error {
	return func(journal vfs.TrashJournal) error {
		msg, err := job.NewMessage(journal)
		if err != nil {
			return err
		}
		_, err = job.System().PushJob(inst, &job.JobRequest{
			WorkerType: "trash-files",
			Message:    msg,
		})
		return err
	}
}

func instanceURL(c echo.Context) string {
	return middlewares.GetInstance(c).PageURL("/", nil)
}

func updateDirCozyMetadata(c echo.Context, dir *vfs.DirDoc) {
	fcm, _ := cozyMetadataFromClaims(c, false)
	if dir.CozyMetadata == nil {
		fcm.CreatedAt = dir.CreatedAt
		fcm.CreatedByApp = ""
		fcm.CreatedByAppVersion = ""
		dir.CozyMetadata = fcm
	} else {
		dir.CozyMetadata.UpdatedAt = fcm.UpdatedAt
		if len(fcm.UpdatedByApps) > 0 {
			dir.CozyMetadata.UpdatedByApp(fcm.UpdatedByApps[0])
		}
	}
}

func updateFileCozyMetadata(c echo.Context, file *vfs.FileDoc, setUploadFields bool) {
	var oldSourceAccount, oldSourceIdentifier string
	fcm, slug := cozyMetadataFromClaims(c, setUploadFields)
	if file.CozyMetadata == nil {
		fcm.CreatedAt = file.CreatedAt
		fcm.CreatedByApp = ""
		fcm.CreatedByAppVersion = ""
		uploadedAt := file.CreatedAt
		fcm.UploadedAt = &uploadedAt
		file.CozyMetadata = fcm
	} else {
		oldSourceAccount = file.CozyMetadata.SourceAccount
		oldSourceIdentifier = file.CozyMetadata.SourceIdentifier
		file.CozyMetadata.UpdatedAt = fcm.UpdatedAt
		if len(fcm.UpdatedByApps) > 0 {
			file.CozyMetadata.UpdatedByApp(fcm.UpdatedByApps[0])
		}
	}

	if setUploadFields {
		if oldSourceAccount == "" && fcm.SourceAccount != "" {
			file.CozyMetadata.SourceAccount = fcm.SourceAccount
			// To ease the transition to cozyMetadata for io.cozy.files, we fill
			// the CreatedByApp for konnectors that updates a file: the stack can
			// recognize that by the presence of the SourceAccount.
			if file.CozyMetadata.CreatedByApp == "" && slug != "" {
				file.CozyMetadata.CreatedByApp = slug
			}
		}
		if oldSourceIdentifier == "" && fcm.SourceIdentifier != "" {
			file.CozyMetadata.SourceIdentifier = fcm.SourceIdentifier
		}
	}
}

func cozyMetadataFromClaims(c echo.Context, setUploadFields bool) (*vfs.FilesCozyMetadata, string) {
	fcm := vfs.NewCozyMetadata(instanceURL(c))

	var slug, version string
	var client map[string]string
	if claims := c.Get("claims"); claims != nil {
		cl := claims.(permission.Claims)
		switch cl.Audience {
		case consts.AppAudience, consts.KonnectorAudience:
			slug = cl.Subject
		case consts.AccessTokenAudience:
			if perms, err := middlewares.GetPermission(c); err == nil {
				if cli, ok := perms.Client.(*oauth.Client); ok {
					slug = oauth.GetLinkedAppSlug(cli.SoftwareID)
					// Special case for cozy-desktop: it is an OAuth app not linked
					// to a web app, so it has no slug, but we still want to keep
					// in cozyMetadata its changes, so we use a fake slug.
					if slug == "" && strings.Contains(cli.SoftwareID, "cozy-desktop") {
						slug = "cozy-desktop"
					}
					version = cli.SoftwareVersion
					client = map[string]string{
						"id":   cli.ID(),
						"kind": cli.ClientKind,
						"name": cli.ClientName,
					}
				}
			}
		}
	}

	if slug != "" {
		fcm.CreatedByApp = slug
		fcm.CreatedByAppVersion = version
		fcm.UpdatedByApps = []*metadata.UpdatedByAppEntry{
			{
				Slug:     slug,
				Version:  version,
				Date:     fcm.UpdatedAt,
				Instance: fcm.CreatedOn,
			},
		}
	}

	if setUploadFields {
		uploadedAt := fcm.CreatedAt
		fcm.UploadedAt = &uploadedAt
		fcm.UploadedOn = fcm.CreatedOn
		if slug != "" {
			fcm.UploadedBy = &vfs.UploadedByEntry{
				Slug:    slug,
				Version: version,
				Client:  client,
			}
		}
	}

	if account := c.QueryParam("SourceAccount"); account != "" {
		fcm.SourceAccount = account
	}
	if id := c.QueryParam("SourceAccountIdentifier"); id != "" {
		fcm.SourceIdentifier = id
	}

	return fcm, slug
}
