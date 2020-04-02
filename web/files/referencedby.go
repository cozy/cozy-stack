package files

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// AddReferencedHandler is the echo.handler for adding referenced_by to
// a file
// POST /files/:file-id/relationships/referenced_by
func AddReferencedHandler(c echo.Context) error {
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

	references, err := jsonapi.BindRelations(c.Request())
	if err != nil {
		return WrapVfsError(err)
	}

	var newRev string
	var refs []couchdb.DocReference
	if dir != nil {
		dir.AddReferencedBy(references...)
		updateDirCozyMetadata(c, dir)
		err = couchdb.UpdateDoc(instance, dir)
		newRev = dir.Rev()
		refs = dir.ReferencedBy
	} else {
		file.AddReferencedBy(references...)
		updateFileCozyMetadata(c, file, false)
		err = couchdb.UpdateDoc(instance, file)
		newRev = file.Rev()
		refs = file.ReferencedBy
	}

	if err != nil {
		return WrapVfsError(err)
	}

	count := len(refs)
	meta := jsonapi.RelationshipMeta{Rev: newRev, Count: &count}

	return jsonapi.DataRelations(c, http.StatusOK, refs, &meta, nil, nil)
}

// RemoveReferencedHandler is the echo.handler for removing referenced_by to
// a file
// DELETE /files/:file-id/relationships/referenced_by
func RemoveReferencedHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	dir, file, err := instance.VFS().DirOrFileByID(fileID)
	if err != nil {
		return WrapVfsError(err)
	}

	err = checkPerm(c, permission.DELETE, nil, file)
	if err != nil {
		return err
	}

	references, err := jsonapi.BindRelations(c.Request())
	if err != nil {
		return WrapVfsError(err)
	}

	var newRev string
	var refs []couchdb.DocReference
	if dir != nil {
		dir.RemoveReferencedBy(references...)
		updateDirCozyMetadata(c, dir)
		err = couchdb.UpdateDoc(instance, dir)
		newRev = dir.Rev()
		refs = dir.ReferencedBy
	} else {
		file.RemoveReferencedBy(references...)
		updateFileCozyMetadata(c, file, false)
		err = couchdb.UpdateDoc(instance, file)
		newRev = file.Rev()
		refs = file.ReferencedBy
	}

	if err != nil {
		return WrapVfsError(err)
	}

	count := len(refs)
	meta := jsonapi.RelationshipMeta{Rev: newRev, Count: &count}

	return jsonapi.DataRelations(c, http.StatusOK, refs, &meta, nil, nil)
}
