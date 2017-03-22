package files

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

// AddReferencedHandler is the echo.handler for adding referenced_by to
// a file
// POST /files/:file-id/relationships/referenced_by
func AddReferencedHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	dir, file, err := instance.VFS().DirOrFileByID(fileID)
	if err != nil {
		return wrapVfsError(err)
	}

	err = checkPerm(c, permissions.PATCH, dir, file)
	if err != nil {
		return err
	}

	if dir != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Cant add references to a folder")
	}
	if file == nil {
		return echo.NewHTTPError(http.StatusNotFound, "File not found")
	}

	references, err := jsonapi.BindRelations(c.Request())
	if err != nil {
		return wrapVfsError(err)
	}

	file.AddReferencedBy(references...)

	err = couchdb.UpdateDoc(instance, file)
	if err != nil {
		return wrapVfsError(err)
	}

	return c.NoContent(http.StatusNoContent)
}

// RemoveReferencedHandler is the echo.handler for removing referenced_by to
// a file
// DELETE /files/:file-id/relationships/referenced_by
func RemoveReferencedHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")

	file, err := instance.VFS().FileByID(fileID)
	if err != nil {
		return wrapVfsError(err)
	}

	err = checkPerm(c, permissions.DELETE, nil, file)
	if err != nil {
		return err
	}

	references, err := jsonapi.BindRelations(c.Request())
	if err != nil {
		return wrapVfsError(err)
	}

	file.RemoveReferencedBy(references...)

	err = couchdb.UpdateDoc(instance, file)
	if err != nil {
		return wrapVfsError(err)
	}

	return c.NoContent(http.StatusNoContent)
}
