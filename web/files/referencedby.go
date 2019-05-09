package files

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
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

	if dir == nil {
		file.AddReferencedBy(references...)
		err = couchdb.UpdateDoc(instance, file)
	} else {
		dir.AddReferencedBy(references...)
		err = couchdb.UpdateDoc(instance, dir)
	}

	if err != nil {
		return WrapVfsError(err)
	}

	return c.NoContent(http.StatusNoContent)
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

	if dir != nil {
		dir.RemoveReferencedBy(references...)
		err = couchdb.UpdateDoc(instance, dir)
	} else {
		file.RemoveReferencedBy(references...)
		err = couchdb.UpdateDoc(instance, file)
	}

	if err != nil {
		return WrapVfsError(err)
	}

	return c.NoContent(http.StatusNoContent)
}
