package files

// Links is used to generate a JSON-API link for the directory (part of
import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/cozy/cozy-stack/pkg/consts"
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
	ref []jsonapi.ResourceIdentifier
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
	var byFetch int
	if count < vfs.IteratorDefaultFetchSize {
		byFetch = int(count)
	}
	if count > maxPerPage {
		count = maxPerPage
	}
	return int(count), &vfs.IteratorOptions{
		ByFetch:  byFetch,
		StartKey: cursorQuery,
	}, nil
}

func newDir(doc *vfs.DirDoc) *dir {
	return &dir{doc: doc}
}

func dirData(c echo.Context, statusCode int, doc *vfs.DirDoc) error {
	var relsData []jsonapi.ResourceIdentifier
	var included []jsonapi.Object

	count, iterOpts, err := paginationConfig(c)
	if err != nil {
		return err
	}

	hasNext := true

	i := middlewares.GetInstance(c)
	iter := doc.ChildrenIterator(i, iterOpts)
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
		var ri jsonapi.ResourceIdentifier
		if d != nil {
			ri = jsonapi.ResourceIdentifier{ID: d.ID(), Type: d.DocType()}
		} else {
			ri = jsonapi.ResourceIdentifier{ID: f.ID(), Type: f.DocType()}
		}
		relsData = append(relsData, ri)
	}

	var parent jsonapi.Relationship
	if doc.ID() != consts.RootDirID {
		parent = jsonapi.Relationship{
			Links: &jsonapi.LinksList{
				Related: "/files/" + doc.DirID,
			},
			Data: jsonapi.ResourceIdentifier{
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
		next := fmt.Sprintf("/files/%s?Page[cursor]=%s&Page[limit]=%d",
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
	var included []jsonapi.Object

	count, iterOpts, err := paginationConfig(c)
	if err != nil {
		return err
	}

	i := middlewares.GetInstance(c)
	iter := doc.ChildrenIterator(i, iterOpts)
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
// Warning: the given document is mutated, the ReferencedBy field is nilified
// so that it not dumped in the json document.
func newFile(doc *vfs.FileDoc) *file {
	ref := doc.ReferencedBy
	doc.ReferencedBy = nil
	return &file{
		doc: doc,
		ref: ref,
	}
}

func fileData(c echo.Context, statusCode int, doc *vfs.FileDoc, links *jsonapi.LinksList) error {
	return jsonapi.Data(c, statusCode, newFile(doc), links)
}

func (d *dir) ID() string                             { return d.doc.ID() }
func (d *dir) Rev() string                            { return d.doc.Rev() }
func (d *dir) SetID(string)                           {}
func (d *dir) SetRev(string)                          {}
func (d *dir) DocType() string                        { return d.doc.DocType() }
func (d *dir) Relationships() jsonapi.RelationshipMap { return d.rel }
func (d *dir) Included() []jsonapi.Object             { return d.included }
func (d *dir) MarshalJSON() ([]byte, error)           { return json.Marshal(d.doc) }
func (d *dir) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/files/" + d.doc.DocID}
}

func (f *file) ID() string      { return f.doc.ID() }
func (f *file) Rev() string     { return f.doc.Rev() }
func (f *file) SetID(string)    {}
func (f *file) SetRev(string)   {}
func (f *file) DocType() string { return f.doc.DocType() }
func (f *file) Relationships() jsonapi.RelationshipMap {
	return jsonapi.RelationshipMap{
		"parent": jsonapi.Relationship{
			Links: &jsonapi.LinksList{
				Related: "/files/" + f.doc.DirID,
			},
			Data: jsonapi.ResourceIdentifier{
				ID:   f.doc.DirID,
				Type: consts.Files,
			},
		},
		"referenced_by": jsonapi.Relationship{
			Links: &jsonapi.LinksList{
				Self: "/files/" + f.doc.ID() + "/relationships/references",
			},
			Data: f.ref,
		},
	}
}
func (f *file) Included() []jsonapi.Object { return []jsonapi.Object{} }
func (f *file) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.doc)
}
func (f *file) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/files/" + f.doc.DocID}
}
