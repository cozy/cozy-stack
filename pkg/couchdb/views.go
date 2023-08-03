package couchdb

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"time"

	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"golang.org/x/sync/errgroup"
)

// View is the map/reduce thing in CouchDB
type View struct {
	Name    string      `json:"-"`
	Doctype string      `json:"-"`
	Map     interface{} `json:"map"`
	Reduce  interface{} `json:"reduce,omitempty"`
	Options interface{} `json:"options,omitempty"`
}

// DesignDoc is the structure if a _design doc containing views
type DesignDoc struct {
	ID    string           `json:"_id,omitempty"`
	Rev   string           `json:"_rev,omitempty"`
	Lang  string           `json:"language"`
	Views map[string]*View `json:"views"`
}

// IndexCreationResponse is the response from couchdb when we create an Index
type IndexCreationResponse struct {
	Result string `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
	Reason string `json:"reason,omitempty"`
	ID     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
}

// DefineViews creates a design doc with some views
func DefineViews(g *errgroup.Group, db prefixer.Prefixer, views []*View) {
	for i := range views {
		view := views[i]
		g.Go(func() error {
			return DefineView(db, view)
		})
	}
}

// DefineView ensure that a view exists, or creates it.
func DefineView(db prefixer.Prefixer, v *View) error {
	id := "_design/" + v.Name
	url := url.PathEscape(id)
	doc := &DesignDoc{
		ID:    id,
		Lang:  "javascript",
		Views: map[string]*View{v.Name: v},
	}
	err := makeRequest(db, v.Doctype, http.MethodPut, url, &doc, nil)
	if IsNoDatabaseError(err) {
		err = CreateDB(db, v.Doctype)
		if err != nil && !IsFileExists(err) {
			if err != nil {
				logger.WithDomain(db.DomainName()).
					Infof("Cannot create view %s %s: cannot create DB - %s",
						db.DBPrefix(), v.Doctype, err)
			}
			return err
		}
		err = makeRequest(db, v.Doctype, http.MethodPut, url, &doc, nil)
	}
	if IsConflictError(err) {
		var old DesignDoc
		err = makeRequest(db, v.Doctype, http.MethodGet, url, nil, &old)
		if err != nil {
			if err != nil {
				logger.WithDomain(db.DomainName()).
					Infof("Cannot create view %s %s: conflict - %s",
						db.DBPrefix(), v.Doctype, err)
			}
			return err
		}
		if !equalViews(&old, doc) {
			doc.Rev = old.Rev
			err = makeRequest(db, v.Doctype, http.MethodPut, url, &doc, nil)
		} else {
			err = nil
		}
	}
	if err != nil {
		logger.WithDomain(db.DomainName()).
			Infof("Cannot create view %s %s: %s", db.DBPrefix(), v.Doctype, err)
	}
	return err
}

// UpdateIndexesAndViews creates views and indexes that are missing or not
// up-to-date.
func UpdateIndexesAndViews(db prefixer.Prefixer, indexes []*mango.Index, views []*View) error {
	g, _ := errgroup.WithContext(context.Background())

	// Load the existing design docs
	idsByDoctype := map[string][]string{}
	for _, view := range views {
		list := idsByDoctype[view.Doctype]
		list = append(list, "_design/"+view.Name)
		idsByDoctype[view.Doctype] = list
	}
	for _, index := range indexes {
		list := idsByDoctype[index.Doctype]
		list = append(list, "_design/"+index.Request.DDoc)
		idsByDoctype[index.Doctype] = list
	}
	ddocsByDoctype := map[string][]*DesignDoc{}
	for doctype, ids := range idsByDoctype {
		req := &AllDocsRequest{Keys: ids, Limit: 10000}
		results := []*DesignDoc{}
		err := GetDesignDocs(db, doctype, req, &results)
		if err != nil {
			continue
		}
		ddocsByDoctype[doctype] = results
	}

	// Define views that don't exist
	for i := range views {
		view := views[i]
		ddoc := &DesignDoc{
			ID:    "_design/" + view.Name,
			Lang:  "javascript",
			Views: map[string]*View{view.Name: view},
		}
		exists := false
		for _, old := range ddocsByDoctype[view.Doctype] {
			if old != nil && equalViews(old, ddoc) {
				exists = true
			}
		}
		if exists {
			continue
		}
		g.Go(func() error {
			return DefineView(db, view)
		})
	}

	// Define indexes that don't exist
	for i := range indexes {
		index := indexes[i]
		ddoc := &DesignDoc{
			ID:   "_design/" + index.Request.DDoc,
			Lang: "query",
		}
		exists := false
		for _, old := range ddocsByDoctype[index.Doctype] {
			if old == nil {
				continue
			}
			name := "undefined"
			for key := range old.Views {
				name = key
			}
			mapFields := map[string]interface{}{}
			defFields := []interface{}{}
			for _, field := range index.Request.Index.Fields {
				mapFields[field] = "asc"
				defFields = append(defFields, field)
			}
			view := &View{
				Name:    name,
				Doctype: index.Doctype,
				Map: map[string]interface{}{
					"fields":                  mapFields,
					"partial_filter_selector": index.Request.Index.PartialFilter,
				},
				Reduce: "_count",
				Options: map[string]interface{}{
					"def": map[string]interface{}{
						"fields": defFields,
					},
				},
			}
			ddoc.Views = map[string]*View{name: view}
			if equalViews(old, ddoc) {
				exists = true
			}
		}
		if exists {
			continue
		}
		g.Go(func() error {
			return DefineIndex(db, index)
		})
	}

	return g.Wait()
}

func equalViews(v1 *DesignDoc, v2 *DesignDoc) bool {
	if v1.Lang != v2.Lang {
		return false
	}
	if len(v1.Views) != len(v2.Views) {
		return false
	}
	for name, view1 := range v1.Views {
		view2, ok := v2.Views[name]
		if !ok {
			return false
		}
		if !reflect.DeepEqual(view1.Map, view2.Map) ||
			!reflect.DeepEqual(view1.Reduce, view2.Reduce) ||
			!reflect.DeepEqual(view1.Options, view2.Options) {
			return false
		}
	}
	return true
}

// ExecView executes the specified view function
func ExecView(db prefixer.Prefixer, view *View, req *ViewRequest, results interface{}) error {
	viewurl := fmt.Sprintf("_design/%s/_view/%s", view.Name, view.Name)
	if req.GroupLevel > 0 {
		req.Group = true
	}
	v, err := req.Values()
	if err != nil {
		return err
	}
	viewurl += "?" + v.Encode()
	if req.Keys != nil {
		return makeRequest(db, view.Doctype, http.MethodPost, viewurl, req, &results)
	}
	err = makeRequest(db, view.Doctype, http.MethodGet, viewurl, nil, &results)
	if IsInternalServerError(err) {
		time.Sleep(1 * time.Second)
		// Retry the error on 500, as it may be just that CouchDB is slow to build the view
		err = makeRequest(db, view.Doctype, http.MethodGet, viewurl, nil, &results)
		if IsInternalServerError(err) {
			logger.
				WithDomain(db.DomainName()).
				WithNamespace("couchdb").
				WithField("critical", "true").
				Errorf("500 on requesting view: %s", err)
		}
	}
	return err
}

// DefineIndexes defines a list of indexes.
func DefineIndexes(g *errgroup.Group, db prefixer.Prefixer, indexes []*mango.Index) {
	for i := range indexes {
		index := indexes[i]
		g.Go(func() error { return DefineIndex(db, index) })
	}
}

// DefineIndex define the index on the doctype database
// see query package on how to define an index
func DefineIndex(db prefixer.Prefixer, index *mango.Index) error {
	_, err := DefineIndexRaw(db, index.Doctype, index.Request)
	if err != nil {
		logger.WithDomain(db.DomainName()).
			Infof("Cannot create index %s %s: %s", db.DBPrefix(), index.Doctype, err)
	}
	return err
}

// DefineIndexRaw defines a index
func DefineIndexRaw(db prefixer.Prefixer, doctype string, index interface{}) (*IndexCreationResponse, error) {
	url := "_index"
	response := &IndexCreationResponse{}
	err := makeRequest(db, doctype, http.MethodPost, url, &index, &response)
	if IsNoDatabaseError(err) {
		if err = CreateDB(db, doctype); err != nil && !IsFileExists(err) {
			return nil, err
		}
		err = makeRequest(db, doctype, http.MethodPost, url, &index, &response)
	}
	// XXX when creating the same index twice at the same time, CouchDB respond
	// with a 500, so let's just retry as a work-around...
	if IsInternalServerError(err) {
		time.Sleep(100 * time.Millisecond)
		err = makeRequest(db, doctype, http.MethodPost, url, &index, &response)
	}
	if err != nil {
		return nil, err
	}
	return response, nil
}
