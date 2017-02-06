package files

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/vfs"
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

	dir, file, err := vfs.GetDirOrFileDoc(instance, fileID, true)
	if err != nil {
		return wrapVfsError(err)
	}

	if dir != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Cant add references to a folder")
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

	return nil
}
