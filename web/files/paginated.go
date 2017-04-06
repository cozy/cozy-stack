package files

// Links is used to generate a JSON-API link for the directory (part of
import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
)

const (
	defPerPage = 30
	maxPerPage = 100
)

type dir struct {
	doc      *vfs.DirDoc
	rel      jsonapi.RelationshipMap
	included []jsonapi.Object
}

type file struct {
	doc *vfs.FileDoc
}

type apiArchive struct {
	*vfs.Archive
}

func paginationConfig(c echo.Context) (int, *vfs.IteratorOptions, error) {
	var count int64
	var err error
	cursorQuery := c.QueryParam("page[cursor]")
	limitQuery := c.QueryParam("page[limit]")
	if limitQuery != "" {
		count, err = strconv.ParseInt(limitQuery, 10, 32)
		if err != nil {
			return 0, nil, err
		}
	} else {
		count = defPerPage
	}
	if count > maxPerPage {
		count = maxPerPage
	}
	return int(count), &vfs.IteratorOptions{
		ByFetch: int(count),
		AfterID: cursorQuery,
	}, nil
}

func newDir(doc *vfs.DirDoc) *dir {
	return &dir{doc: doc}
}

func dirData(c echo.Context, statusCode int, doc *vfs.DirDoc) error {
	relsData := make([]couchdb.DocReference, 0)
	included := make([]jsonapi.Object, 0)

	count, iterOpts, err := paginationConfig(c)
	if err != nil {
		return err
	}

	hasNext := true

	i := middlewares.GetInstance(c)
	iter := i.VFS().DirIterator(doc, iterOpts)
	for i := 0; i < count; i++ {
		d, f, err := iter.Next()
		if err == vfs.ErrIteratorDone {
			hasNext = false
			break
		}
		if err != nil {
			return err
		}
		if d != nil {
			included = append(included, newDir(d))
		} else {
			included = append(included, newFile(f))
		}
		var ri couchdb.DocReference
		if d != nil {
			ri = couchdb.DocReference{ID: d.ID(), Type: d.DocType()}
		} else {
			ri = couchdb.DocReference{ID: f.ID(), Type: f.DocType()}
		}
		relsData = append(relsData, ri)
	}

	var parent jsonapi.Relationship
	if doc.ID() != consts.RootDirID {
		parent = jsonapi.Relationship{
			Links: &jsonapi.LinksList{
				Related: "/files/" + doc.DirID,
			},
			Data: couchdb.DocReference{
				ID:   doc.DirID,
				Type: consts.Files,
			},
		}
	}
	rel := jsonapi.RelationshipMap{
		"parent":   parent,
		"contents": jsonapi.Relationship{Data: relsData},
	}

	var links *jsonapi.LinksList
	if hasNext && len(included) > 0 {
		next := fmt.Sprintf("/files/%s?page[cursor]=%s&page[limit]=%d",
			doc.DocID, included[len(included)-1].ID(), count)
		links = &jsonapi.LinksList{Next: next}
	}

	dir := &dir{
		doc:      doc,
		rel:      rel,
		included: included,
	}
	return jsonapi.Data(c, statusCode, dir, links)
}

func dirDataList(c echo.Context, statusCode int, doc *vfs.DirDoc) error {
	included := make([]jsonapi.Object, 0)

	count, iterOpts, err := paginationConfig(c)
	if err != nil {
		return err
	}

	i := middlewares.GetInstance(c)
	iter := i.VFS().DirIterator(doc, iterOpts)
	for i := 0; i < count; i++ {
		d, f, err := iter.Next()
		if err == vfs.ErrIteratorDone {
			break
		}
		if err != nil {
			return err
		}
		if d != nil {
			included = append(included, newDir(d))
		} else {
			included = append(included, newFile(f))
		}
	}
	return jsonapi.DataList(c, statusCode, included, nil)
}

// newFile creates an instance of file struct from a vfs.FileDoc document.
func newFile(doc *vfs.FileDoc) *file {
	return &file{doc}
}

func fileData(c echo.Context, statusCode int, doc *vfs.FileDoc, links *jsonapi.LinksList) error {
	return jsonapi.Data(c, statusCode, newFile(doc), links)
}

var (
	_ jsonapi.Object = (*apiArchive)(nil)
	_ jsonapi.Object = (*dir)(nil)
	_ jsonapi.Object = (*file)(nil)
)

func (d *dir) ID() string                             { return d.doc.ID() }
func (d *dir) Rev() string                            { return d.doc.Rev() }
func (d *dir) SetID(id string)                        { d.doc.SetID(id) }
func (d *dir) SetRev(rev string)                      { d.doc.SetRev(rev) }
func (d *dir) DocType() string                        { return d.doc.DocType() }
func (d *dir) Relationships() jsonapi.RelationshipMap { return d.rel }
func (d *dir) Included() []jsonapi.Object             { return d.included }
func (d *dir) MarshalJSON() ([]byte, error)           { return json.Marshal(d.doc) }
func (d *dir) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/files/" + d.doc.DocID}
}

func (a *apiArchive) Relationships() jsonapi.RelationshipMap { return nil }
func (a *apiArchive) Included() []jsonapi.Object             { return nil }
func (a *apiArchive) MarshalJSON() ([]byte, error)           { return json.Marshal(a.Archive) }
func (a *apiArchive) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/files/archive/" + a.Secret}
}

func (f *file) ID() string        { return f.doc.ID() }
func (f *file) Rev() string       { return f.doc.Rev() }
func (f *file) SetID(id string)   { f.doc.SetID(id) }
func (f *file) SetRev(rev string) { f.doc.SetRev(rev) }
func (f *file) DocType() string   { return f.doc.DocType() }
func (f *file) Relationships() jsonapi.RelationshipMap {
	return jsonapi.RelationshipMap{
		"parent": jsonapi.Relationship{
			Links: &jsonapi.LinksList{
				Related: "/files/" + f.doc.DirID,
			},
			Data: couchdb.DocReference{
				ID:   f.doc.DirID,
				Type: consts.Files,
			},
		},
		"referenced_by": jsonapi.Relationship{
			Links: &jsonapi.LinksList{
				Self: "/files/" + f.doc.ID() + "/relationships/references",
			},
			Data: f.doc.ReferencedBy,
		},
	}
}
func (f *file) Included() []jsonapi.Object { return []jsonapi.Object{} }
func (f *file) MarshalJSON() ([]byte, error) {
	ref := f.doc.ReferencedBy
	f.doc.ReferencedBy = nil
	res, err := json.Marshal(f.doc)
	f.doc.ReferencedBy = ref
	return res, err
}
func (f *file) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/files/" + f.doc.DocID}
}
