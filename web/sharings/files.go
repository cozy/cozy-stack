package sharings

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"reflect"
	"strconv"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/sharings"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

// We need custom handlers for files for several reasons:
// - to be able to put the shared directories in the "Shared with me" directory
// - to manage conflicts
// - fix some tricky edge cases (see RemoveDocumentIfNotShared)

func creationWithIDHandler(c echo.Context) error {
	ins := middlewares.GetInstance(c)

	sharingID := c.QueryParam(consts.QueryParamSharingID)
	if sharingID == "" {
		return jsonapi.BadRequest(errors.New("Missing sharing id"))
	}

	sharing, errf := sharings.FindSharing(ins, sharingID)
	if errf != nil {
		return errf
	}
	slug := sharing.AppSlug

	dirID := c.QueryParam(consts.QueryParamDirID)
	var err error
	if dirID == "" {
		dirID, err = sharings.RetrieveApplicationDestinationDirID(ins, slug,
			consts.Files)
		if err != nil {
			return err
		}
	}

	switch c.QueryParam(consts.QueryParamType) {
	case consts.FileType:
		err = createFileWithIDHandler(c, ins, dirID)
	case consts.DirType:
		err = createDirWithIDHandler(c, ins, dirID)
	default:
		return files.ErrDocTypeInvalid
	}

	return err
}

func createDirWithIDHandler(c echo.Context, ins *instance.Instance, dirID string) error {
	fs := ins.VFS()
	name := c.QueryParam(consts.QueryParamName)
	id := c.Param("docid")

	// TODO handle name collision.
	newDir, err := vfs.NewDirDoc(fs, name, dirID, nil)
	if err != nil {
		return err
	}

	newDir.SetID(id)

	refBy := c.QueryParam(consts.QueryParamReferencedBy)
	if refBy != "" {
		var refs = []couchdb.DocReference{}
		b := []byte(refBy)
		if err = json.Unmarshal(b, &refs); err != nil {
			return err
		}
		newDir.ReferencedBy = refs
	}

	createdAt, err := time.Parse(time.RFC1123,
		c.QueryParam(consts.QueryParamCreatedAt))
	if err != nil {
		return err
	}
	newDir.CreatedAt = createdAt

	updatedAt, err := time.Parse(time.RFC1123,
		c.QueryParam(consts.QueryParamUpdatedAt))
	if err != nil {
		return err
	}
	newDir.UpdatedAt = updatedAt

	if err = permissions.AllowVFS(c, "POST", newDir); err != nil {
		return err
	}

	// Caveat: this function can be called not just for creation. If one were to
	// reshare the same directory we would end up here and `DirByID` won't
	// return an error since the directory actually exists.
	// So if that situation happens it means the directory is to be updated and,
	// in order to not lose information, we need to manually merge the oldDoc
	// and newDoc references and tags. We assume by default that the values of
	// newDoc are the correct ones.
	oldDoc, err := fs.DirByID(newDir.ID())
	if err == nil {
		ins.Logger().Debugf("[sharings] Modification detected instead of "+
			"creation: %s", newDir.ID())
		newDir.Tags = mergeTags(newDir.Tags, oldDoc.Tags)
		newDir.ReferencedBy = mergeReferencedBy(newDir.ReferencedBy,
			oldDoc.ReferencedBy)
		newDir.SetRev(oldDoc.Rev())

		errm := modifyDirOrFileMetadata(c, fs, newDir, nil, &vfs.DocPatch{})
		if errm != nil {
			return errm
		}

		return c.NoContent(http.StatusOK)
	}

	return fs.CreateDir(newDir)
}

func createFileWithIDHandler(c echo.Context, ins *instance.Instance, dirID string) error {
	fs := ins.VFS()
	name := c.QueryParam(consts.QueryParamName)

	newFile, err := files.FileDocFromReq(c, name, dirID, nil)
	if err != nil {
		return err
	}

	newFile.SetID(c.Param("docid"))

	refBy := c.QueryParam(consts.QueryParamReferencedBy)
	if refBy != "" {
		var refs = []couchdb.DocReference{}
		b := []byte(refBy)
		if err = json.Unmarshal(b, &refs); err != nil {
			return err
		}
		newFile.ReferencedBy = refs
	}

	createdAt, err := time.Parse(time.RFC1123,
		c.QueryParam(consts.QueryParamCreatedAt))
	if err != nil {
		return err
	}
	newFile.CreatedAt = createdAt

	updatedAt, err := time.Parse(time.RFC1123,
		c.QueryParam(consts.QueryParamUpdatedAt))
	if err != nil {
		return err
	}
	newFile.UpdatedAt = updatedAt

	if err = permissions.AllowVFS(c, "POST", newFile); err != nil {
		return err
	}

	// Caveat: this function can be called not just for creation. If one were to
	// reshare the same file we would end up here and `FileByID` won't return an
	// error since the file actually exists.
	// So if that situation happens it means the file is to be updated and, in
	// order to not lose information, we need to manually merge the oldDoc and
	// newDoc metadata, references and tags.
	// We assume by default that the values of newDoc are the correct ones.
	oldFile, err := fs.FileByID(newFile.ID())
	if err != nil {
		oldFile = nil
	} else {
		ins.Logger().Debugf("[sharings] Modification detected instead of "+
			"creation: %s", newFile.ID())

		newFile.Metadata = mergeMetadata(newFile.Metadata, oldFile.Metadata)
		newFile.ReferencedBy = mergeReferencedBy(newFile.ReferencedBy,
			oldFile.ReferencedBy)
		newFile.Tags = mergeTags(newFile.Tags, oldFile.Tags)
		newFile.SetRev(oldFile.Rev())

		err = modifyDirOrFileMetadata(c, fs, nil, newFile, &vfs.DocPatch{})
		if err != nil {
			return err
		}
	}

	file, err := fs.CreateFile(newFile, oldFile)
	if err != nil {
		return err
	}

	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
		if err != nil {
			return
		}
		err = c.JSON(http.StatusOK, nil)
	}()

	_, err = io.Copy(file, c.Request().Body)
	return err
}

func updateFile(c echo.Context) error {
	fs := middlewares.GetInstance(c).VFS()
	olddoc, err := fs.FileByID(c.Param("docid"))
	if err != nil {
		return err
	}

	newdoc, err := files.FileDocFromReq(
		c,
		c.QueryParam(consts.QueryParamName),
		olddoc.DirID,
		olddoc.Tags,
	)
	newdoc.ReferencedBy = olddoc.ReferencedBy

	updatedAt, err := time.Parse(time.RFC1123,
		c.QueryParam(consts.QueryParamUpdatedAt))
	if err != nil {
		return err
	}
	newdoc.UpdatedAt = updatedAt

	if err = files.CheckIfMatch(c, olddoc.Rev()); err != nil {
		return err
	}

	if err = permissions.AllowVFS(c, permissions.PUT, olddoc); err != nil {
		return err
	}

	// If the permission is on the ID so the newdoc has to have the same ID or
	// the permission check will fail.
	newdoc.SetID(olddoc.ID())
	if err = permissions.AllowVFS(c, permissions.PUT, newdoc); err != nil {
		return err
	}

	file, err := fs.CreateFile(newdoc, olddoc)
	if err != nil {
		return err
	}

	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
		if err != nil {
			return
		}
		err = c.JSON(http.StatusOK, nil)
	}()

	_, err = io.Copy(file, c.Request().Body)
	return err
}

func patchDirOrFile(c echo.Context) error {
	ins := middlewares.GetInstance(c)
	ins.Logger().Debugf("[sharings] Patching %s: %s", consts.Files,
		c.Param("docid"))

	instance := middlewares.GetInstance(c)
	var patch vfs.DocPatch

	sharingID := c.QueryParam(consts.QueryParamSharingID)
	if sharingID == "" {
		return jsonapi.BadRequest(errors.New("Missing sharing id"))
	}

	_, err := jsonapi.Bind(c.Request(), &patch)
	if err != nil {
		return jsonapi.BadJSON()
	}

	patch.RestorePath = nil

	dirDoc, fileDoc, err := instance.VFS().DirOrFileByID(c.Param("docid"))
	if err != nil {
		return err
	}

	var rev string
	if dirDoc != nil {
		// Safeguard for the date in case of incorrect UpdatedAt from the remote
		if patch.UpdatedAt.Before(dirDoc.CreatedAt) {
			*patch.UpdatedAt = dirDoc.UpdatedAt
		}
		if *patch.DirID == "" {
			*patch.DirID = dirDoc.DirID
		}
		rev = dirDoc.Rev()
	} else {
		// Safeguard for the date in case of incorrect UpdatedAt from the remote
		if patch.UpdatedAt.Before(fileDoc.CreatedAt) {
			*patch.UpdatedAt = fileDoc.UpdatedAt
		}
		if *patch.DirID == "" {
			*patch.DirID = fileDoc.DirID
		}
		rev = fileDoc.Rev()
	}

	if errc := files.CheckIfMatch(c, rev); err != nil {
		return errc
	}

	err = modifyDirOrFileMetadata(c, instance.VFS(), dirDoc, fileDoc, &patch)
	if err != nil {
		return err
	}

	return c.NoContent(http.StatusOK)
}

func modifyDirOrFileMetadata(c echo.Context, fs vfs.VFS, dirDoc *vfs.DirDoc, fileDoc *vfs.FileDoc, patch *vfs.DocPatch) error {
	if dirDoc != nil {
		if err := permissions.AllowVFS(c, permissions.PATCH, dirDoc); err != nil {
			return err
		}
		_, err := vfs.ModifyDirMetadata(fs, dirDoc, patch)
		return err
	}
	if err := permissions.AllowVFS(c, permissions.PATCH, fileDoc); err != nil {
		return err
	}
	_, err := vfs.ModifyFileMetadata(fs, fileDoc, patch)
	return err
}

// This function calls the handler from web/files to remove the references, and
// then remove the file if it is no longer shared and if the user is not the
// original sharer.
//
// The permissions are checked in the handler from web/files.
func removeReferences(c echo.Context) error {
	err := files.RemoveReferencedHandler(c)
	if err != nil {
		return err
	}

	sharerStr := c.QueryParam(consts.QueryParamSharer)
	sharer, err := strconv.ParseBool(sharerStr)
	if err != nil {
		return err
	}

	if !sharer {
		ins := middlewares.GetInstance(c)
		err = sharings.RemoveDocumentIfNotShared(ins, consts.Files, c.Param("file-id"))
		if err != nil {
			return err
		}
	}

	return c.NoContent(http.StatusNoContent)
}

func trashHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("docid")
	dir, file, err := instance.VFS().DirOrFileByID(fileID)
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
		if err = permissions.AllowVFS(c, permissions.DELETE, dir); err != nil {
			return err
		}
		_, err = vfs.TrashDir(instance.VFS(), dir)
		if err != nil {
			return err
		}
		return nil
	}

	if err = permissions.AllowVFS(c, permissions.DELETE, file); err != nil {
		return err
	}
	_, err = vfs.TrashFile(instance.VFS(), file)
	if err != nil {
		return err
	}
	return nil
}

func mergeMetadata(newMeta, oldMeta vfs.Metadata) vfs.Metadata {
	if newMeta == nil {
		return oldMeta
	}

	res := vfs.Metadata{}
	for newKey, newValue := range newMeta {
		res[newKey] = newValue
	}

	for oldKey, oldValue := range oldMeta {
		if _, present := newMeta[oldKey]; !present {
			res[oldKey] = oldValue
		}
	}

	return res
}

func mergeReferencedBy(newRefs, oldRefs []couchdb.DocReference) []couchdb.DocReference {
	if len(newRefs) == 0 {
		return oldRefs
	}

	res := make([]couchdb.DocReference, len(newRefs))
	copy(res, newRefs)

	for _, oldReference := range oldRefs {
		var exists bool
		for _, newReference := range newRefs {
			if reflect.DeepEqual(newReference, oldReference) {
				exists = true
				break
			}
		}

		if !exists {
			res = append(res, oldReference)
		}
	}

	return res
}

func mergeTags(newTags, oldTags []string) []string {
	if len(newTags) == 0 {
		return oldTags
	}

	res := make([]string, len(newTags))
	copy(res, newTags)

	for _, oldTag := range oldTags {
		var exists bool
		for _, newTag := range newTags {
			if newTag == oldTag {
				exists = true
				break
			}
		}

		if !exists {
			res = append(res, oldTag)
		}
	}

	return res
}
