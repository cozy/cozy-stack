package data

import (
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

func addReferencesHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	references, err := jsonapi.BindRelations(c.Request())
	if err != nil {
		return err
	}

	docRef := jsonapi.ResourceIdentifier{
		Type: c.Get("doctype").(string),
		ID:   c.Param("docid"),
	}

	for _, fRef := range references {
		file, err := vfs.GetFileDoc(instance, fRef.ID)
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
