package files

// Links is used to generate a JSON-API link for the directory (part of
import (
	"encoding/json"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

const (
	defPerPage = 30
)

type dir struct {
	doc      *vfs.DirDoc
	rel      jsonapi.RelationshipMap
	included []jsonapi.Object
}

type file struct {
	doc      *vfs.FileDoc
	instance *instance.Instance
	versions []*vfs.Version
	// fileJSON is used for marshaling to JSON and we keep a reference here to
	// avoid many allocations.
	jsonDoc     *fileJSON
	includePath bool
}

type apiArchive struct {
	*vfs.Archive
}

type apiMetadata struct {
	*vfs.Metadata
	secret string
}

func newDir(doc *vfs.DirDoc) *dir {
	return &dir{doc: doc}
}

func getDirData(c echo.Context, doc *vfs.DirDoc) (int, couchdb.Cursor, []vfs.DirOrFileDoc, error) {
	instance := middlewares.GetInstance(c)
	fs := instance.VFS()

	cursor, err := jsonapi.ExtractPaginationCursor(c, defPerPage, 0)
	if err != nil {
		return 0, nil, nil, err
	}

	count, err := fs.DirLength(doc)
	if err != nil {
		return 0, nil, nil, err
	}

	// Hide the trash folder when listing the root directory.
	var limit int
	if doc.ID() == consts.RootDirID {
		if count > 0 {
			count--
		}
		switch c := cursor.(type) {
		case *couchdb.StartKeyCursor:
			limit = c.Limit
			if c.NextKey == nil {
				c.Limit++
			}
		case *couchdb.SkipCursor:
			limit = c.Limit
			if c.Skip == 0 {
				c.Limit++
			} else {
				c.Skip++
			}
		}
	}

	children, err := fs.DirBatch(doc, cursor)
	if err != nil {
		return 0, nil, nil, err
	}

	if doc.ID() == consts.RootDirID {
		switch c := cursor.(type) {
		case *couchdb.StartKeyCursor:
			c.Limit = limit
		case *couchdb.SkipCursor:
			c.Limit = limit
			c.Skip--
		}
	}

	return count, cursor, children, nil
}

func dirData(c echo.Context, statusCode int, doc *vfs.DirDoc) error {
	instance := middlewares.GetInstance(c)
	count, cursor, children, err := getDirData(c, doc)
	if err != nil {
		return err
	}

	relsData := make([]couchdb.DocReference, 0)
	included := make([]jsonapi.Object, 0)

	for _, child := range children {
		if child.ID() == consts.TrashDirID {
			continue
		}
		relsData = append(relsData, couchdb.DocReference{ID: child.ID(), Type: child.DocType()})
		d, f := child.Refine()
		if d != nil {
			included = append(included, newDir(d))
		} else {
			included = append(included, NewFile(f, instance))
		}
	}

	var parent jsonapi.Relationship
	if doc.ID() != consts.RootDirID {
		parent = jsonapi.Relationship{
			Links: &jsonapi.LinksList{
				Self: "/files/" + doc.DirID,
			},
			Data: couchdb.DocReference{
				ID:   doc.DirID,
				Type: consts.Files,
			},
		}
	}
	rel := jsonapi.RelationshipMap{
		"parent": parent,
		"contents": jsonapi.Relationship{
			Meta: &jsonapi.Meta{Count: &count},
			Links: &jsonapi.LinksList{
				Self: "/files/" + doc.DocID + "/relationships/contents",
			},
			Data: relsData},
		"referenced_by": jsonapi.Relationship{
			Links: &jsonapi.LinksList{
				Self: "/files/" + doc.ID() + "/relationships/references",
			},
			Data: doc.ReferencedBy,
		},
	}

	var links jsonapi.LinksList
	if cursor.HasMore() {
		params, err := jsonapi.PaginationCursorToParams(cursor)
		if err != nil {
			return err
		}
		next := "/files/" + doc.DocID + "/relationships/contents?" + params.Encode()
		rel["contents"].Links.Next = next
		links.Next = "/files/" + doc.DocID + "?" + params.Encode()
	}

	d := &dir{
		doc:      doc,
		rel:      rel,
		included: included,
	}

	return jsonapi.Data(c, statusCode, d, &links)
}

func dirDataList(c echo.Context, statusCode int, doc *vfs.DirDoc) error {
	instance := middlewares.GetInstance(c)
	count, cursor, children, err := getDirData(c, doc)
	if err != nil {
		return err
	}

	included := make([]jsonapi.Object, 0)
	for _, child := range children {
		if child.ID() == consts.TrashDirID {
			continue
		}
		d, f := child.Refine()
		if d != nil {
			included = append(included, newDir(d))
		} else {
			included = append(included, NewFile(f, instance))
		}
	}

	var links jsonapi.LinksList
	if cursor.HasMore() {
		params, err := jsonapi.PaginationCursorToParams(cursor)
		if err != nil {
			return err
		}
		next := c.Request().URL.Path + "?" + params.Encode()
		links.Next = next
	}

	return jsonapi.DataListWithTotal(c, statusCode, count, included, &links)
}

// NewFile creates an instance of file struct from a vfs.FileDoc document.
func NewFile(doc *vfs.FileDoc, i *instance.Instance) *file {
	return &file{doc, i, nil, &fileJSON{}, false}
}

// FileData returns a jsonapi representation of the given file.
func FileData(c echo.Context, statusCode int, doc *vfs.FileDoc, withVersions bool, links *jsonapi.LinksList) error {
	instance := middlewares.GetInstance(c)
	f := NewFile(doc, instance)
	if withVersions {
		if versions, err := vfs.VersionsFor(instance, doc.ID()); err == nil {
			f.versions = versions
		}
	}
	return jsonapi.Data(c, statusCode, f, links)
}

var (
	_ jsonapi.Object = (*apiArchive)(nil)
	_ jsonapi.Object = (*apiMetadata)(nil)
	_ jsonapi.Object = (*dir)(nil)
	_ jsonapi.Object = (*file)(nil)
)

func (d *dir) ID() string                             { return d.doc.ID() }
func (d *dir) Rev() string                            { return d.doc.Rev() }
func (d *dir) SetID(id string)                        { d.doc.SetID(id) }
func (d *dir) SetRev(rev string)                      { d.doc.SetRev(rev) }
func (d *dir) DocType() string                        { return d.doc.DocType() }
func (d *dir) Clone() couchdb.Doc                     { cloned := *d; return &cloned }
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

func (m *apiMetadata) ID() string                             { return m.secret }
func (m *apiMetadata) Rev() string                            { return "" }
func (m *apiMetadata) SetID(id string)                        { m.secret = id }
func (m *apiMetadata) SetRev(rev string)                      {}
func (m *apiMetadata) DocType() string                        { return consts.FilesMetadata }
func (m *apiMetadata) Clone() couchdb.Doc                     { cloned := *m; return &cloned }
func (m *apiMetadata) Relationships() jsonapi.RelationshipMap { return nil }
func (m *apiMetadata) Included() []jsonapi.Object             { return nil }
func (m *apiMetadata) MarshalJSON() ([]byte, error)           { return json.Marshal(m.Metadata) }
func (m *apiMetadata) Links() *jsonapi.LinksList              { return nil }

type fileJSON struct {
	*vfs.FileDoc
	// XXX Hide the internal_vfs_id and referenced_by
	InternalID   *interface{} `json:"internal_vfs_id,omitempty"`
	ReferencedBy *interface{} `json:"referenced_by,omitempty"`
	// Include the path if asked for
	Fullpath string `json:"path,omitempty"`
}

func (f *file) ID() string         { return f.doc.ID() }
func (f *file) Rev() string        { return f.doc.Rev() }
func (f *file) SetID(id string)    { f.doc.SetID(id) }
func (f *file) SetRev(rev string)  { f.doc.SetRev(rev) }
func (f *file) DocType() string    { return f.doc.DocType() }
func (f *file) Clone() couchdb.Doc { cloned := *f; return &cloned }
func (f *file) Relationships() jsonapi.RelationshipMap {
	rels := jsonapi.RelationshipMap{
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
	if len(f.versions) > 0 {
		data := make([]couchdb.DocReference, len(f.versions))
		for i, version := range f.versions {
			data[i] = couchdb.DocReference{
				ID:   version.DocID,
				Type: consts.FilesVersions,
			}
		}
		rels["old_versions"] = jsonapi.Relationship{
			Data: data,
		}
	}
	return rels
}

func (f *file) Included() []jsonapi.Object {
	var included []jsonapi.Object
	for _, version := range f.versions {
		included = append(included, version)
	}
	return included
}

func (f *file) MarshalJSON() ([]byte, error) {
	f.jsonDoc.FileDoc = f.doc
	if f.includePath {
		f.jsonDoc.Fullpath, _ = f.doc.Path(nil)
	}
	res, err := json.Marshal(f.jsonDoc)
	return res, err
}

func (f *file) Links() *jsonapi.LinksList {
	links := jsonapi.LinksList{Self: "/files/" + f.doc.DocID}
	if f.doc.Class == "image" {
		if secret, err := vfs.GetStore().AddThumb(f.instance, f.doc.DocID); err == nil {
			links.Small = "/files/" + f.doc.DocID + "/thumbnails/" + secret + "/small"
			links.Medium = "/files/" + f.doc.DocID + "/thumbnails/" + secret + "/medium"
			links.Large = "/files/" + f.doc.DocID + "/thumbnails/" + secret + "/large"
		}
	}
	return &links
}

func (f *file) IncludePath(fp vfs.FilePather) {
	f.includePath = true
	_, _ = f.doc.Path(fp)
}
