package sharings

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

// SharedWithMeDirName is the name of the directory that will contain all shared
// files.
// TODO Put in a locale aware constant.
const SharedWithMeDirName = "Shared With Me"

func creationWithIDHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	fs := instance.VFS()

	err := createDirForSharing(fs, consts.SharedWithMeDirID, "")
	if err != nil {
		return err
	}

	switch c.QueryParam(consts.QueryParamType) {
	case consts.FileType:
		err = createFileWithIDHandler(c, fs)
	case consts.DirType:
		err = createDirWithIDHandler(c, fs)
	default:
		return files.ErrDocTypeInvalid
	}

	return err
}

func createDirWithIDHandler(c echo.Context, fs vfs.VFS) error {
	name := c.QueryParam(consts.QueryParamName)
	id := c.Param("docid")

	// TODO handle name collision.
	doc, err := vfs.NewDirDoc(fs, name, "", nil)
	if err != nil {
		return err
	}

	doc.DirID = c.QueryParam(consts.QueryParamDirID)
	doc.SetID(id)

	createdAt, err := time.Parse(time.RFC1123,
		c.QueryParam(consts.QueryParamCreatedAt))
	if err != nil {
		return err
	}
	doc.CreatedAt = createdAt

	updatedAt, err := time.Parse(time.RFC1123,
		c.QueryParam(consts.QueryParamUpdatedAt))
	if err != nil {
		return err
	}
	doc.UpdatedAt = updatedAt

	if err = permissions.AllowVFS(c, "POST", doc); err != nil {
		return err
	}

	return fs.CreateDir(doc)
}

func createFileWithIDHandler(c echo.Context, fs vfs.VFS) error {
	name := c.QueryParam(consts.QueryParamName)

	doc, err := files.FileDocFromReq(c, name, "", nil)
	if err != nil {
		return err
	}

	doc.SetID(c.Param("docid"))
	doc.DirID = c.QueryParam(consts.QueryParamDirID)

	refBy := c.QueryParam(consts.QueryParamReferencedBy)
	if refBy != "" {
		var refs = []couchdb.DocReference{}
		b := []byte(refBy)
		if err = json.Unmarshal(b, &refs); err != nil {
			return err
		}
		doc.ReferencedBy = refs
	}

	createdAt, err := time.Parse(time.RFC1123,
		c.QueryParam(consts.QueryParamCreatedAt))
	if err != nil {
		return err
	}
	doc.CreatedAt = createdAt

	updatedAt, err := time.Parse(time.RFC1123,
		c.QueryParam(consts.QueryParamUpdatedAt))
	if err != nil {
		return err
	}
	doc.UpdatedAt = updatedAt

	if err = permissions.AllowVFS(c, "POST", doc); err != nil {
		return err
	}

	file, err := fs.CreateFile(doc, nil)
	if err != nil {
		return err
	}

	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
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

	// The permission is on the ID so the newdoc has to have the same ID or
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
	instance := middlewares.GetInstance(c)
	var patch vfs.DocPatch

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
		*patch.DirID = dirDoc.DirID
		rev = dirDoc.Rev()
	} else {
		*patch.DirID = fileDoc.DirID
		rev = fileDoc.Rev()
	}

	if errc := files.CheckIfMatch(c, rev); err != nil {
		return errc
	}

	if dirDoc != nil {
		_, err = vfs.ModifyDirMetadata(instance.VFS(), dirDoc, &patch)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, nil)
	}

	_, err = vfs.ModifyFileMetadata(instance.VFS(), fileDoc, &patch)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, nil)
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
		_, errt := vfs.TrashDir(instance.VFS(), dir)
		if errt != nil {
			return err
		}
		return nil
	}

	_, errt := vfs.TrashFile(instance.VFS(), file)
	if errt != nil {
		return err
	}
	return nil
}

// This function either creates the "Shared With Me" directory at the root of
// the cozy or creates the directory with the given name and id under the
// "Shared With Me" directory.
//
// If a name isn't provided then the id will be used as a replacement.
func createDirForSharing(fs vfs.VFS, id, name string) error {
	if _, errd := fs.DirByID(id); errd == nil {
		return nil
	}

	var dirID string
	if id == consts.SharedWithMeDirID {
		dirID = ""
		name = SharedWithMeDirName
	} else {
		if _, errd := fs.DirByID(consts.SharedWithMeDirID); errd != nil {
			errc := createDirForSharing(fs, consts.SharedWithMeDirID, "")
			if errc != nil {
				return errc
			}
		}
		dirID = consts.SharedWithMeDirID
	}

	if name == "" {
		name = id
	}

	dirDoc, err := vfs.NewDirDoc(fs, name, dirID, nil)
	if err != nil {
		return err
	}

	dirDoc.SetID(id)
	t := time.Now()
	dirDoc.CreatedAt = t
	dirDoc.UpdatedAt = t

	return fs.CreateDir(dirDoc)
}
