package files

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
)

const maxRefLimit = 100

func rawMessageToObject(i *instance.Instance, bb json.RawMessage) (jsonapi.Object, error) {
	var dof vfs.DirOrFileDoc
	err := json.Unmarshal(bb, &dof)
	if err != nil {
		return nil, err
	}
	d, f := dof.Refine()
	if d != nil {
		return newDir(d), nil
	}

	return newFile(f, i), nil
}

// ListReferencesHandler list all files referenced by a doc
// GET /data/:type/:id/relationships/references
// Beware, this is actually used in the web/data Routes
func ListReferencesHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	doctype := c.Get("doctype").(string)
	id := c.Param("docid")
	include := c.QueryParam("include")
	includeDocs := include == "files"

	if err := permissions.AllowTypeAndID(c, permissions.GET, doctype, id); err != nil {
		return err
	}

	cursor, err := jsonapi.ExtractPaginationCursor(c, maxRefLimit)
	if err != nil {
		return err
	}

	key := []string{doctype, id}
	reqCount := &couchdb.ViewRequest{Key: key, Reduce: true}

	var resCount couchdb.ViewResponse
	err = couchdb.ExecView(instance, consts.FilesReferencedByView, reqCount, &resCount)
	if err != nil {
		return err
	}

	count := 0
	if len(resCount.Rows) > 0 {
		count = int(resCount.Rows[0].Value.(float64))
	}

	req := &couchdb.ViewRequest{Key: key, IncludeDocs: includeDocs, Reduce: false}
	cursor.ApplyTo(req)

	var res couchdb.ViewResponse
	err = couchdb.ExecView(instance, consts.FilesReferencedByView, req, &res)
	if err != nil {
		return err
	}

	cursor.UpdateFrom(&res)

	var links = &jsonapi.LinksList{}
	if cursor.HasMore() {
		params, err2 := jsonapi.PaginationCursorToParams(cursor)
		if err2 != nil {
			return err2
		}
		links.Next = fmt.Sprintf("%s?%s",
			c.Request().URL.Path, params.Encode())
	}

	var refs = make([]couchdb.DocReference, len(res.Rows))
	var docs []jsonapi.Object
	if includeDocs {
		docs = make([]jsonapi.Object, len(res.Rows))
	}

	for i, row := range res.Rows {
		refs[i] = couchdb.DocReference{
			ID:   row.ID,
			Type: consts.Files,
		}

		if includeDocs {
			docs[i], err = rawMessageToObject(instance, *row.Doc)
			if err != nil {
				return err
			}
		}
	}

	return jsonapi.DataRelations(c, http.StatusOK, refs, count, links, docs)
}

// AddReferencesHandler add some files references to a doc
// POST /data/:type/:id/relationships/references
// Beware, this is actually used in the web/data Routes
func AddReferencesHandler(c echo.Context) error {
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
		dir, file, err := instance.VFS().DirOrFileByID(fRef.ID)
		if err != nil {
			return err
		}
		if file == nil {
			dir.AddReferencedBy(docRef)
			err = couchdb.UpdateDoc(instance, dir)
		} else {
			file.AddReferencedBy(docRef)
			err = couchdb.UpdateDoc(instance, file)
		}
		if err != nil {
			return err
		}
	}

	return c.NoContent(204)
}

// RemoveReferencesHandler remove some files references from a doc
// DELETE /data/:type/:id/relationships/references
// Beware, this is actually used in the web/data Routes
func RemoveReferencesHandler(c echo.Context) error {
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
		dir, file, err := instance.VFS().DirOrFileByID(fRef.ID)
		if err != nil {
			return err
		}
		if file == nil {
			dir.RemoveReferencedBy(docRef)
			err = couchdb.UpdateDoc(instance, dir)
		} else {
			file.RemoveReferencedBy(docRef)
			err = couchdb.UpdateDoc(instance, file)
		}
		if err != nil {
			return err
		}
	}

	return c.NoContent(204)
}
