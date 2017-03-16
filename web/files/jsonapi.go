package files

// Links is used to generate a JSON-API link for the directory (part of
import (
	"encoding/json"
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

// type file struct {
// 	doc *vfs.FileDoc
// 	rel jsonapi.RelationshipMap
// }

type dir struct {
	doc      *vfs.DirDoc
	rel      jsonapi.RelationshipMap
	included []jsonapi.Object
}

func paginationConfig(c echo.Context) (int, *vfs.IteratorOptions, error) {
	var count int64
	var err error
	afterQuery := c.QueryParam("After")
	countQuery := c.QueryParam("Count")
	if countQuery != "" {
		count, err = strconv.ParseInt(countQuery, 10, 32)
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
		StartKey: afterQuery,
	}, nil
}

func dirData(c echo.Context, statusCode int, doc *vfs.DirDoc) error {
	var relsData []jsonapi.ResourceIdentifier
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
			included = append(included, d)
		} else {
			included = append(included, f)
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
	dir := &dir{
		doc:      doc,
		rel:      rel,
		included: included,
	}
	return jsonapi.Data(c, statusCode, dir, nil)
}

func dirDataList(c echo.Context, statusCode int, doc *vfs.DirDoc) error {
	var included []jsonapi.Object
	i := middlewares.GetInstance(c)
	iter := doc.ChildrenIterator(i, &vfs.IteratorOptions{
		ByFetch: 10,
	})
	for {
		d, f, err := iter.Next()
		if err == vfs.ErrIteratorDone {
			break
		}
		if err != nil {
			return err
		}
		if d != nil {
			included = append(included, d)
		} else {
			included = append(included, f)
		}
	}
	return jsonapi.DataList(c, statusCode, included, nil)
}

func (d *dir) ID() string      { return d.doc.ID() }
func (d *dir) Rev() string     { return d.doc.Rev() }
func (d *dir) SetID(string)    {}
func (d *dir) SetRev(string)   {}
func (d *dir) DocType() string { return d.doc.DocType() }
func (d *dir) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.doc)
}

// jsonapi.Object interface)
func (d *dir) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/files/" + d.doc.DocID}
}

// Relationships is used to generate the content relationship in JSON-API format
// (part of the jsonapi.Object interface)
func (d *dir) Relationships() jsonapi.RelationshipMap {
	return d.rel
}

// Included is part of the jsonapi.Object interface
func (d *dir) Included() []jsonapi.Object {
	return d.included
}
