package files

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// AddNotSynchronizedOn is the echo.handler for adding not_synchronized_on to
// a directory
// POST /files/:file-id/relationships/not_synchronized_on
func AddNotSynchronizedOn(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")
	dir, err := instance.VFS().DirByID(fileID)
	if err != nil {
		return WrapVfsError(err)
	}

	if err = middlewares.AllowVFS(c, permission.PATCH, dir); err != nil {
		return err
	}

	references, err := jsonapi.BindRelations(c.Request())
	if err != nil {
		return WrapVfsError(err)
	}

	dir.AddNotSynchronizedOn(references...)
	updateDirCozyMetadata(c, dir)
	if err = couchdb.UpdateDoc(instance, dir); err != nil {
		return WrapVfsError(err)
	}

	refs := dir.NotSynchronizedOn
	count := len(refs)
	meta := jsonapi.Meta{Rev: dir.Rev(), Count: &count}
	return jsonapi.DataRelations(c, http.StatusOK, refs, &meta, nil, nil)
}

// RemoveNotSynchronizedOn is the echo.handler for removing not_synchronized_on to
// a directory
// DELETE /files/:file-id/relationships/not_synchronized_on
func RemoveNotSynchronizedOn(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	fileID := c.Param("file-id")
	dir, err := instance.VFS().DirByID(fileID)
	if err != nil {
		return WrapVfsError(err)
	}

	if err = middlewares.AllowVFS(c, permission.PATCH, dir); err != nil {
		return err
	}

	references, err := jsonapi.BindRelations(c.Request())
	if err != nil {
		return WrapVfsError(err)
	}

	dir.RemoveNotSynchronizedOn(references...)
	updateDirCozyMetadata(c, dir)
	if err = couchdb.UpdateDoc(instance, dir); err != nil {
		return WrapVfsError(err)
	}

	refs := dir.NotSynchronizedOn
	count := len(refs)
	meta := jsonapi.Meta{Rev: dir.Rev(), Count: &count}
	return jsonapi.DataRelations(c, http.StatusOK, refs, &meta, nil, nil)
}
