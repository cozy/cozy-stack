package data

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

func listReferencesHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	doctype := c.Get("doctype").(string)
	id := c.Param("docid")

	if err := permissions.AllowTypeAndID(c, permissions.GET, doctype, id); err != nil {
		return err
	}

	refs, err := vfs.FilesReferencedBy(instance, doctype, id)
	if err != nil {
		return err
	}

	return jsonapi.DataRelations(c, http.StatusOK, refs)
}

func addReferencesHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	doctype := c.Get("doctype").(string)
	id := c.Param("docid")

	references, err := jsonapi.BindRelations(c.Request())
	if err != nil {
		return err
	}

	docRef := jsonapi.ResourceIdentifier{
		Type: doctype,
		ID:   id,
	}

	if err := permissions.AllowTypeAndID(c, permissions.PUT, doctype, id); err != nil {
		return err
	}

	for _, fRef := range references {
		file, err := instance.VFS().FileByID(fRef.ID)
		if err != nil {
			return err
		}
		file.AddReferencedBy(docRef)
		err = couchdb.UpdateDoc(instance, file)
		if err != nil {
			return err
		}
	}

	return c.NoContent(204)
}

func removeReferencesHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	doctype := c.Get("doctype").(string)
	id := c.Param("docid")

	references, err := jsonapi.BindRelations(c.Request())
	if err != nil {
		return err
	}

	docRef := jsonapi.ResourceIdentifier{
		Type: doctype,
		ID:   id,
	}

	if err := permissions.AllowTypeAndID(c, permissions.DELETE, doctype, id); err != nil {
		return err
	}

	for _, fRef := range references {
		file, err := instance.VFS().FileByID(fRef.ID)
		if err != nil {
			return err
		}
		file.RemoveReferencedBy(docRef)
		err = couchdb.UpdateDoc(instance, file)
		if err != nil {
			return err
		}
	}

	return c.NoContent(204)
}
