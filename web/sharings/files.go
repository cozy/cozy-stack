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

// TODO Support sharing of recursive directories. For now all directories go
// to /Shared With Me/
func creationWithIDHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	fs := instance.VFS()

	_, err := fs.DirByID(consts.SharedWithMeDirID)
	if err != nil {
		err = createSharedWithMeDir(fs)
		if err != nil {
			return err
		}
	}

	switch c.QueryParam("Type") {
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
	name := c.QueryParam("Name")
	id := c.Param("docid")
	date := c.Request().Header.Get("Date")

	// TODO handle name collision.
	doc, err := vfs.NewDirDoc(fs, name, "", nil)
	if err != nil {
		return err
	}

	doc.DirID = consts.SharedWithMeDirID
	doc.SetID(id)

	if date != "" {
		if t, errt := time.Parse(time.RFC1123, date); errt == nil {
			doc.CreatedAt = t
			doc.UpdatedAt = t
		}
	}

	if err = permissions.AllowVFS(c, "POST", doc); err != nil {
		return err
	}

	return fs.CreateDir(doc)
}

func createFileWithIDHandler(c echo.Context, fs vfs.VFS) error {
	name := c.QueryParam("Name")

	doc, err := files.FileDocFromReq(c, name, "", nil)
	if err != nil {
		return err
	}

	doc.SetID(c.Param("docid"))
	doc.DirID = consts.SharedWithMeDirID

	refBy := c.QueryParam("Referenced_by")
	if refBy != "" {
		var refs = []couchdb.DocReference{}
		b := []byte(refBy)
		if err = json.Unmarshal(b, &refs); err != nil {
			return err
		}
		doc.ReferencedBy = refs
	}

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

func createSharedWithMeDir(fs vfs.VFS) error {
	// TODO Put "Shared With Me" in a local-aware constant.
	dirDoc, err := vfs.NewDirDoc(fs, "Shared With Me", "", nil)
	if err != nil {
		return err
	}

	dirDoc.SetID(consts.SharedWithMeDirID)
	t := time.Now()
	dirDoc.CreatedAt = t
	dirDoc.UpdatedAt = t

	return fs.CreateDir(dirDoc)
}

func updateFile(c echo.Context) error {
	fs := middlewares.GetInstance(c).VFS()
	olddoc, err := fs.FileByID(c.Param("docid"))
	if err != nil {
		return err
	}

	newdoc, err := files.FileDocFromReq(
		c,
		c.QueryParam("Name"),
		// TODO Handle dir hierarchy within a sharing and stop putting
		// everything in "Shared With Me".
		consts.SharedWithMeDirID,
		olddoc.Tags,
	)
	newdoc.ReferencedBy = olddoc.ReferencedBy

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

	// TODO When supported re-apply hierarchy here.

	*patch.DirID = consts.SharedWithMeDirID
	patch.RestorePath = nil

	dirDoc, fileDoc, err := instance.VFS().DirOrFileByID(c.Param("docid"))
	if err != nil {
		return err
	}

	var rev string
	if dirDoc != nil {
		rev = dirDoc.Rev()
	} else {
		rev = fileDoc.Rev()
	}

	if err := files.CheckIfMatch(c, rev); err != nil {
		return err
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
