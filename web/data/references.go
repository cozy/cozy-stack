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

	page, err := jsonapi.ExtractPagination(c, maxRefLimit)
	if err != nil {
		return err
	}

	req := &couchdb.ViewRequest{
		Key: []string{doctype, id},
	}

	req = (&couchdb.Cursor{
		Limit:     page.Limit,
		NextKey:   req.Key,
		NextDocID: page.Cursor,
	}).ApplyTo(req)

	var res couchdb.ViewResponse
	err = couchdb.ExecView(instance, consts.FilesReferencedByView, req, &res)
	if err != nil {
		return err
	}

	var links *jsonapi.LinksList
	if len(res.Rows) > page.Limit {
		cursor := couchdb.GetNextCursor(&res)
		nextLink := fmt.Sprintf("%s?page[cursor]=%s&page[limit]=%d",
			c.Request().URL.Path, cursor.NextDocID, page.Limit)
		links = &jsonapi.LinksList{Next: nextLink}
	}

	var out = make([]jsonapi.ResourceIdentifier, len(res.Rows))
	for i, row := range res.Rows {
		out[i] = jsonapi.ResourceIdentifier{
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
