package data

import (
	"fmt"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

const maxRefLimit = 30

func listReferencesHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	doctype := c.Get("doctype").(string)
	id := c.Param("docid")

	if err := permissions.AllowTypeAndID(c, permissions.GET, doctype, id); err != nil {
		return err
	}

	cursor, err := jsonapi.ExtractPaginationCursor(c, maxRefLimit)
	if err != nil {
		return err
	}

	req := &couchdb.ViewRequest{
		Key: []string{doctype, id},
	}

	cursor.ApplyTo(req)

	var res couchdb.ViewResponse
	err = couchdb.ExecView(instance, consts.FilesReferencedByView, req, &res)
	if err != nil {
		return err
	}

	cursor.UpdateFrom(&res)

	var links = &jsonapi.LinksList{}
	if !cursor.Done {
		params, err := jsonapi.PaginationCursorToParams(cursor)
		if err != nil {
			return err
		}
		links.Next = fmt.Sprintf("%s?%s",
			c.Request().URL.Path, params.Encode())
	}

	var out = make([]couchdb.DocReference, len(res.Rows))
	for i, row := range res.Rows {
		out[i] = couchdb.DocReference{
			ID:   row.ID,
			Type: consts.Files,
		}
	}

	return jsonapi.DataRelations(c, http.StatusOK, out, links)
}

func addReferencesHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	doctype := c.Get("doctype").(string)
	id := c.Param("docid")

	references, err := jsonapi.BindRelations(c.Request())
	if err != nil {
		return err
	}

	docRef := couchdb.DocReference{
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

	docRef := couchdb.DocReference{
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
