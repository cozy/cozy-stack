package files

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

const (
	defaultRefsPerPage = 100
	maxRefsPerPage     = 1000
)

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

	return NewFile(f, i), nil
}

// ListReferencesHandler list all files referenced by a doc
// GET /data/:type/:id/relationships/references
// Beware, this is actually used in the web/data Routes
func ListReferencesHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	doctype := c.Get("doctype").(string)
	id := getDocID(c)
	include := c.QueryParam("include")
	includeDocs := include == "files"

	if err := middlewares.AllowTypeAndID(c, permission.GET, doctype, id); err != nil {
		if middlewares.AllowWholeType(c, permission.GET, consts.Files) != nil {
			return err
		}
	}

	cursor, err := jsonapi.ExtractPaginationCursor(c, defaultRefsPerPage, maxRefsPerPage)
	if err != nil {
		return err
	}

	key := []string{doctype, id}
	reqCount := &couchdb.ViewRequest{Key: key, Reduce: true}

	var resCount couchdb.ViewResponse
	err = couchdb.ExecView(instance, couchdb.FilesReferencedByView, reqCount, &resCount)
	if err != nil {
		return err
	}

	count := 0
	if len(resCount.Rows) > 0 {
		count = int(resCount.Rows[0].Value.(float64))
	}

	// XXX Some references can contains %2f instead of in the id (legacy),
	// and to preserve compatibility, we try to find those documents if no
	// documents with the correct reference are found.
	if count == 0 && strings.Contains(id, "/") {
		key[1] = c.Param("docid")
		err = couchdb.ExecView(instance, couchdb.FilesReferencedByView, reqCount, &resCount)
		if err == nil && len(resCount.Rows) > 0 {
			count = int(resCount.Rows[0].Value.(float64))
		}
	}
	meta := &jsonapi.RelationshipMeta{Count: &count}

	sort := c.QueryParam("sort")
	descending := strings.HasPrefix(sort, "-")
	start := key
	end := []string{key[0], key[1], couchdb.MaxString}
	if descending {
		start, end = end, start
	}
	var view *couchdb.View
	switch sort {
	case "", "id", "-id":
		view = couchdb.FilesReferencedByView
	case "datetime", "-datetime":
		view = couchdb.ReferencedBySortedByDatetimeView
	default:
		return jsonapi.BadRequest(errors.New("Invalid sort parameter"))
	}

	req := &couchdb.ViewRequest{
		StartKey:    start,
		EndKey:      end,
		IncludeDocs: includeDocs,
		Reduce:      false,
		Descending:  descending,
	}
	cursor.ApplyTo(req)

	var res couchdb.ViewResponse
	err = couchdb.ExecView(instance, view, req, &res)
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
			docs[i], err = rawMessageToObject(instance, row.Doc)
			if err != nil {
				return err
			}
		}
	}

	return jsonapi.DataRelations(c, http.StatusOK, refs, meta, links, docs)
}

// AddReferencesHandler add some files references to a doc
// POST /data/:type/:id/relationships/references
// Beware, this is actually used in the web/data Routes
func AddReferencesHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	doctype := c.Get("doctype").(string)
	id := getDocID(c)

	references, err := jsonapi.BindRelations(c.Request())
	if err != nil {
		return WrapVfsError(err)
	}

	docRef := couchdb.DocReference{
		Type: doctype,
		ID:   id,
	}

	if err = middlewares.AllowTypeAndID(c, permission.PUT, doctype, id); err != nil {
		if middlewares.AllowWholeType(c, permission.PATCH, consts.Files) != nil {
			return err
		}
	}
	docs := make([]interface{}, len(references))
	oldDocs := make([]interface{}, len(references))

	for i, fRef := range references {
		dir, file, errd := instance.VFS().DirOrFileByID(fRef.ID)
		if errd != nil {
			return WrapVfsError(errd)
		}
		if dir != nil {
			oldDir := dir.Clone()
			dir.AddReferencedBy(docRef)
			updateDirCozyMetadata(c, dir)
			docs[i] = dir
			oldDocs[i] = oldDir
		} else {
			oldFile := file.Clone()
			file.AddReferencedBy(docRef)
			updateFileCozyMetadata(c, file, false)
			docs[i] = file
			oldDocs[i] = oldFile
		}
	}
	// Use bulk update for better performances
	err = couchdb.BulkUpdateDocs(instance, consts.Files, docs, oldDocs)
	if err != nil {
		return WrapVfsError(err)
	}
	return c.NoContent(204)
}

// RemoveReferencesHandler remove some files references from a doc
// DELETE /data/:type/:id/relationships/references
// Beware, this is actually used in the web/data Routes
func RemoveReferencesHandler(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	doctype := c.Get("doctype").(string)
	id := getDocID(c)

	references, err := jsonapi.BindRelations(c.Request())
	if err != nil {
		return WrapVfsError(err)
	}

	docRef := couchdb.DocReference{
		Type: doctype,
		ID:   id,
	}

	// XXX References with an ID that contains a / could have it encoded as %2F
	// (legacy). We delete the references for both versions to preserve
	// compatibility.
	var altRef *couchdb.DocReference
	if strings.Contains(id, "/") {
		altRef = &couchdb.DocReference{
			Type: doctype,
			ID:   c.Param("docid"),
		}
	}

	if err := middlewares.AllowTypeAndID(c, permission.DELETE, doctype, id); err != nil {
		if middlewares.AllowWholeType(c, permission.PATCH, consts.Files) != nil {
			return err
		}
	}

	for _, fRef := range references {
		dir, file, err := instance.VFS().DirOrFileByID(fRef.ID)
		if err != nil {
			return WrapVfsError(err)
		}
		if dir != nil {
			dir.RemoveReferencedBy(docRef)
			if altRef != nil {
				dir.RemoveReferencedBy(*altRef)
			}
			updateDirCozyMetadata(c, dir)
			err = couchdb.UpdateDoc(instance, dir)
		} else {
			file.RemoveReferencedBy(docRef)
			if altRef != nil {
				file.RemoveReferencedBy(*altRef)
			}
			updateFileCozyMetadata(c, file, false)
			err = couchdb.UpdateDoc(instance, file)
		}
		if err != nil {
			return WrapVfsError(err)
		}
	}

	return c.NoContent(204)
}

func getDocID(c echo.Context) string {
	id := c.Param("docid")
	if docid, err := url.PathUnescape(id); err == nil {
		return docid
	}
	return id
}
