package couchdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/google/go-querystring/query"
)

// AllDocsRequest is used to build a _all_docs request
type AllDocsRequest struct {
	Descending    bool     `url:"descending,omitempty"`
	Limit         int      `url:"limit,omitempty"`
	Skip          int      `url:"skip,omitempty"`
	StartKey      string   `url:"startkey,omitempty"`
	StartKeyDocID string   `url:"startkey_docid,omitempty"`
	EndKey        string   `url:"endkey,omitempty"`
	EndKeyDocID   string   `url:"endkey_docid,omitempty"`
	Keys          []string `url:"keys,omitempty"`
}

// AllDocsResponse is the response we receive from an _all_docs request
type AllDocsResponse struct {
	Offset    int `json:"offset"`
	TotalRows int `json:"total_rows"`
	Rows      []struct {
		ID  string          `json:"id"`
		Doc json.RawMessage `json:"doc"`
	} `json:"rows"`
}

// IDRev is used for the payload of POST _bulk_get
type IDRev struct {
	ID  string `json:"id"`
	Rev string `json:"rev,omitempty"`
}

// BulkGetResponse is the response we receive from a _bulk_get request
type BulkGetResponse struct {
	Results []struct {
		Docs []struct {
			OK map[string]interface{} `json:"ok"`
		} `json:"docs"`
	} `json:"results"`
}

// CountAllDocs returns the number of documents of the given doctype.
func CountAllDocs(db Database, doctype string) (int, error) {
	var response AllDocsResponse
	url := "_all_docs?limit=0"
	err := makeRequest(db, doctype, http.MethodGet, url, nil, &response)
	if err != nil {
		return 0, err
	}
	return response.TotalRows, nil
}

// GetAllDocs returns all documents of a specified doctype. It filters
// out the possible _design document.
func GetAllDocs(db Database, doctype string, req *AllDocsRequest, results interface{}) (err error) {
	var v url.Values
	if req != nil {
		v, err = req.Values()
		if err != nil {
			return err
		}
	} else {
		v = make(url.Values)
	}
	v.Add("include_docs", "true")
	var response AllDocsResponse
	if req == nil || len(req.Keys) == 0 {
		url := "_all_docs?" + v.Encode()
		err = makeRequest(db, doctype, http.MethodGet, url, nil, &response)
	} else {
		v.Del("keys")
		url := "_all_docs?" + v.Encode()
		body := struct {
			Keys []string `json:"keys"`
		}{
			Keys: req.Keys,
		}
		err = makeRequest(db, doctype, http.MethodPost, url, body, &response)
	}
	if err != nil {
		return err
	}

	var docs []json.RawMessage
	for _, row := range response.Rows {
		if !strings.HasPrefix(row.ID, "_design") {
			docs = append(docs, row.Doc)
		}
	}
	// TODO: better way to unmarshal returned data. For now we re-
	// marshal the doc fields a a json array before unmarshalling it
	// again...
	data, err := json.Marshal(docs)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, results)
}

// ForeachDocs traverse all the documents from the given database with the
// specified doctype and calls a function for each document.
func ForeachDocs(db Database, doctype string, fn func(id string, doc json.RawMessage) error) error {
	var startKey string
	limit := 100
	for {
		skip := 0
		if startKey != "" {
			skip = 1
		}
		req := &AllDocsRequest{
			StartKeyDocID: startKey,
			Skip:          skip,
			Limit:         limit,
		}
		v, err := query.Values(req)
		if err != nil {
			return err
		}
		v.Add("include_docs", "true")

		var res AllDocsResponse
		url := "_all_docs?" + v.Encode()
		err = makeRequest(db, doctype, http.MethodGet, url, nil, &res)
		if err != nil {
			return err
		}

		var count int
		startKey = ""
		for _, row := range res.Rows {
			if !strings.HasPrefix(row.ID, "_design") {
				if err = fn(row.ID, row.Doc); err != nil {
					return err
				}
				startKey = row.ID
				count++
			}
		}
		if count == 0 || len(res.Rows) < limit {
			break
		}
	}

	return nil
}

// BulkGetDocs returns the documents with the given id at the given revision
func BulkGetDocs(db Database, doctype string, payload []IDRev) ([]map[string]interface{}, error) {
	path := "_bulk_get?revs=true"
	body := struct {
		Docs []IDRev `json:"docs"`
	}{
		Docs: payload,
	}
	var response BulkGetResponse
	err := makeRequest(db, doctype, http.MethodPost, path, body, &response)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]interface{}, 0, len(response.Results))
	for _, r := range response.Results {
		for _, doc := range r.Docs {
			if doc.OK != nil {
				results = append(results, doc.OK)
			}
		}
	}
	return results, nil
}

// BulkUpdateDocs is used to update several docs in one call, as a bulk.
// olddocs parameter is used for realtime / event triggers.
func BulkUpdateDocs(db Database, doctype string, docs, olddocs []interface{}) error {
	if len(docs) == 0 {
		return nil
	}
	body := struct {
		Docs []interface{} `json:"docs"`
	}{
		Docs: docs,
	}
	var res []UpdateResponse
	if err := makeRequest(db, doctype, http.MethodPost, "_bulk_docs", body, &res); err != nil {
		return err
	}
	if len(res) != len(docs) {
		return errors.New("BulkUpdateDoc receive an unexpected number of responses")
	}
	for i, doc := range docs {
		if d, ok := doc.(Doc); ok {
			d.SetRev(res[i].Rev)
			if old, ok := olddocs[i].(Doc); ok {
				RTEvent(db, realtime.EventUpdate, d, old)
			} else {
				RTEvent(db, realtime.EventUpdate, d, nil)
			}
		}
	}
	return nil
}

// BulkDeleteDocs is used to delete serveral documents in one call.
func BulkDeleteDocs(db Database, doctype string, docs []Doc) error {
	if len(docs) == 0 {
		return nil
	}
	body := struct {
		Docs []json.RawMessage `json:"docs"`
	}{
		Docs: make([]json.RawMessage, 0, len(docs)),
	}
	for _, doc := range docs {
		body.Docs = append(body.Docs, json.RawMessage(
			fmt.Sprintf(`{"_id":"%s","_rev":"%s","_deleted":true}`, doc.ID(), doc.Rev()),
		))
	}
	var res []UpdateResponse
	if err := makeRequest(db, doctype, http.MethodPost, "_bulk_docs", body, &res); err != nil {
		return err
	}
	for i, doc := range docs {
		if d, ok := doc.(Doc); ok {
			d.SetRev(res[i].Rev)
			RTEvent(db, realtime.EventDelete, d, nil)
		}
	}
	return nil
}

// BulkForceUpdateDocs is used to update several docs in one call, and to force
// the revisions history. It is used by replications.
func BulkForceUpdateDocs(db Database, doctype string, docs []map[string]interface{}) error {
	if len(docs) == 0 {
		return nil
	}
	body := struct {
		NewEdits bool                     `json:"new_edits"`
		Docs     []map[string]interface{} `json:"docs"`
	}{
		NewEdits: false,
		Docs:     docs,
	}
	// XXX CouchDB returns just an empty array when new_edits is false, so we
	// ignore the response
	return makeRequest(db, doctype, http.MethodPost, "_bulk_docs", body, nil)
}
