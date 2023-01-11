package couchdb

import (
	"context"
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// GetLocal fetch a local document from CouchDB
// http://docs.couchdb.org/en/stable/api/local.html#get--db-_local-docid
func GetLocal(ctx context.Context, db prefixer.Prefixer, doctype, id string) (map[string]interface{}, error) {
	var out map[string]interface{}
	u := "_local/" + url.PathEscape(id)
	if err := makeRequest(ctx, db, doctype, http.MethodGet, u, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// PutLocal will put a local document in CouchDB.
// Note that you should put the last revision in `doc` to avoid conflicts.
func PutLocal(ctx context.Context, db prefixer.Prefixer, doctype, id string, doc map[string]interface{}) error {
	u := "_local/" + url.PathEscape(id)
	var out UpdateResponse
	if err := makeRequest(ctx, db, doctype, http.MethodPut, u, doc, &out); err != nil {
		return err
	}
	doc["_rev"] = out.Rev
	return nil
}

// DeleteLocal will delete a local document in CouchDB.
func DeleteLocal(db prefixer.Prefixer, doctype, id string) error {
	u := "_local/" + url.PathEscape(id)
	return makeRequest(context.TODO(), db, doctype, http.MethodDelete, u, nil, nil)
}
