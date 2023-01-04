package data

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

var client = &http.Client{}

const Type = "io.cozy.events"
const ID = "4521C325F6478E45"

var testInstance *instance.Instance
var token string

var ts *httptest.Server

type stackUpdateResponse struct {
	ID      string          `json:"id"`
	Rev     string          `json:"rev"`
	Type    string          `json:"type"`
	Ok      bool            `json:"ok"`
	Deleted bool            `json:"deleted"`
	Error   string          `json:"error"`
	Reason  string          `json:"reason"`
	Data    couchdb.JSONDoc `json:"data"`
}

// Test for having not the same ID in document and URL

// Test for having an inexisting id at all

type M map[string]interface{}
type S []interface{}
type indexCreationResponse struct {
	Result string `json:"result"`
	Error  string `json:"error"`
	Reason string `json:"reason"`
	ID     string `json:"id"`
	Name   string `json:"name"`
}

func TestData(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile()
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "data_test")
	testInstance = setup.GetTestInstance()
	scope := "io.cozy.doctypes io.cozy.files io.cozy.events " +
		"io.cozy.anothertype io.cozy.nottype"

	_, token = setup.GetTestClient(scope)
	ts = setup.GetTestServer("/data", Routes)

	_ = couchdb.ResetDB(testInstance, Type)
	_ = couchdb.CreateNamedDoc(testInstance, &couchdb.JSONDoc{
		Type: Type,
		M: map[string]interface{}{
			"_id":  ID,
			"test": "testvalue",
		},
	})

	t.Run("SuccessGet", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/data/"+Type+"/"+ID, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		out, res, err := doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, "200 OK", res.Status, "should get a 200")
		if assert.Contains(t, out, "test") {
			assert.Equal(t, out["test"], "testvalue", "should give the same doc")
		}
	})

	t.Run("GetForMissingDoc", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/data/no.such.doctype/id", nil)
		_, res, err := doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, 401, res.StatusCode)

		req, _ = http.NewRequest("GET", ts.URL+"/data/"+Type+"/no.such.id", nil)
		_, res, err = doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, 401, res.StatusCode)

		req, _ = http.NewRequest("GET", ts.URL+"/data/"+Type+"/no-such-id", nil)
		req.Header.Add("Authorization", "Bearer "+token)
		_, res, err = doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, 404, res.StatusCode)
	})

	t.Run("GetWithSlash", func(t *testing.T) {
		_ = couchdb.CreateNamedDoc(testInstance, &couchdb.JSONDoc{
			Type: Type, M: map[string]interface{}{
				"_id":  "with/slash",
				"test": "valueslash",
			}})

		req, _ := http.NewRequest("GET", ts.URL+"/data/"+Type+"/with%2Fslash", nil)
		req.Header.Add("Authorization", "Bearer "+token)
		out, res, err := doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, "200 OK", res.Status, "should get a 200")
		if assert.Contains(t, out, "test") {
			assert.Equal(t, out["test"], "valueslash", "should give the same doc")
		}
	})

	t.Run("WrongDoctype", func(t *testing.T) {
		_ = couchdb.DeleteDB(testInstance, "io.cozy.nottype")

		req, _ := http.NewRequest("GET", ts.URL+"/data/io.cozy.nottype/"+ID, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		out, res, err := doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, "404 Not Found", res.Status, "should get a 404")
		if assert.Contains(t, out, "error") {
			assert.Equal(t, "not_found", out["error"], "should give a json name")
		}
		if assert.Contains(t, out, "reason") {
			assert.Equal(t, "wrong_doctype", out["reason"], "should give a reason")
		}
	})

	t.Run("UnderscoreName", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/data/"+Type+"/_foo", nil)
		req.Header.Add("Authorization", "Bearer "+token)
		_, res, err := doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
	})

	t.Run("VFSDoctype", func(t *testing.T) {
		in := jsonReader(&map[string]interface{}{
			"wrong-vfs": "structure",
		})
		req, _ := http.NewRequest("POST", ts.URL+"/data/io.cozy.files/", in)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		out, res, err := doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, "403 Forbidden", res.Status, "should get a 403")
		if assert.Contains(t, out, "error") {
			assert.Contains(t, out["error"], "reserved", "should give a clear reason")
		}
	})

	t.Run("WrongID", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/data/"+Type+"/NOTID", nil)
		req.Header.Add("Authorization", "Bearer "+token)
		out, res, err := doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, "404 Not Found", res.Status, "should get a 404")
		if assert.Contains(t, out, "error") {
			assert.Equal(t, "not_found", out["error"], "should give a json name")
		}
		if assert.Contains(t, out, "reason") {
			assert.Equal(t, "missing", out["reason"], "should give a reason")
		}
	})

	t.Run("SuccessCreateKnownDoctype", func(t *testing.T) {
		in := jsonReader(&map[string]interface{}{
			"somefield": "avalue",
		})
		var sur stackUpdateResponse
		req, _ := http.NewRequest("POST", ts.URL+"/data/"+Type+"/", in)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		_, res, err := doRequest(req, &sur)
		assert.NoError(t, err)
		assert.Equal(t, "201 Created", res.Status, "should get a 201")
		assert.Equal(t, sur.Ok, true, "ok is true")
		assert.NotContains(t, sur.ID, "/", "id is simple uuid")
		assert.Equal(t, sur.Type, Type, "type is correct")
		assert.NotEmpty(t, sur.Rev, "rev at top level (couchdb compatibility)")
		assert.Equal(t, sur.ID, sur.Data.ID(), "id is simple uuid")
		assert.Equal(t, sur.Type, sur.Data.Type, "type is correct")
		assert.Equal(t, sur.Rev, sur.Data.Rev(), "rev is correct")
		assert.Equal(t, "avalue", sur.Data.Get("somefield"), "content is correct")
	})

	t.Run("SuccessCreateUnknownDoctype", func(t *testing.T) {
		in := jsonReader(&map[string]interface{}{
			"somefield": "avalue",
		})
		var sur stackUpdateResponse
		type2 := "io.cozy.anothertype"
		req, _ := http.NewRequest("POST", ts.URL+"/data/"+type2+"/", in)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		_, res, err := doRequest(req, &sur)
		assert.NoError(t, err)
		assert.Equal(t, "201 Created", res.Status, "should get a 201")
		assert.Equal(t, sur.Ok, true, "ok is true")
		assert.NotContains(t, sur.ID, "/", "id is simple uuid")
		assert.Equal(t, sur.Type, type2, "type is correct")
		assert.NotEmpty(t, sur.Rev, "rev at top level (couchdb compatibility)")
		assert.Equal(t, sur.ID, sur.Data.ID(), "in doc id is correct")
		assert.Equal(t, sur.Type, sur.Data.Type, "in doc type is correct")
		assert.Equal(t, sur.Rev, sur.Data.Rev(), "in doc rev is correct")
		assert.Equal(t, "avalue", sur.Data.Get("somefield"), "content is correct")
	})

	t.Run("WrongCreateWithID", func(t *testing.T) {
		in := jsonReader(&map[string]interface{}{
			"_id":       "this-should-not-be-an-id",
			"somefield": "avalue",
		})
		req, _ := http.NewRequest("POST", ts.URL+"/data/"+Type+"/", in)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		_, res, err := doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
	})

	t.Run("SuccessUpdate", func(t *testing.T) {
		// Get revision
		doc := getDocForTest()
		url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID()

		// update it
		in := jsonReader(&map[string]interface{}{
			"_id":       doc.ID(),
			"_rev":      doc.Rev(),
			"test":      doc.Get("test"),
			"somefield": "anewvalue",
		})
		req, _ := http.NewRequest("PUT", url, in)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		var out stackUpdateResponse
		_, res, err := doRequest(req, &out)
		assert.NoError(t, err)
		assert.Equal(t, "200 OK", res.Status, "should get a 201")
		assert.Empty(t, out.Error, "there is no error")
		assert.Equal(t, out.ID, doc.ID(), "id has not changed")
		assert.Equal(t, out.Ok, true, "ok is true")
		assert.NotEmpty(t, out.Rev, "there is a rev")
		assert.NotEqual(t, out.Rev, doc.Rev(), "rev has changed")
		assert.Equal(t, out.ID, out.Data.ID(), "in doc id is simple uuid")
		assert.Equal(t, out.Type, out.Data.Type, "in doc type is correct")
		assert.Equal(t, out.Rev, out.Data.Rev(), "in doc rev is correct")
		assert.Equal(t, "anewvalue", out.Data.Get("somefield"), "content has changed")
	})

	t.Run("WrongIDInDocUpdate", func(t *testing.T) {
		// Get revision
		doc := getDocForTest()
		// update it
		in := jsonReader(&map[string]interface{}{
			"_id":       "this is not the id in the URL",
			"_rev":      doc.Rev(),
			"test":      doc.M["test"],
			"somefield": "anewvalue",
		})
		url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID()
		req, _ := http.NewRequest("PUT", url, in)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		_, res, err := doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, "400 Bad Request", res.Status, "should get a 404")
	})

	t.Run("CreateDocWithAFixedID", func(t *testing.T) {
		// update it
		in := jsonReader(&map[string]interface{}{
			"test":      "value",
			"somefield": "anewvalue",
		})
		url := ts.URL + "/data/" + Type + "/specific-id"
		req, _ := http.NewRequest("PUT", url, in)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		var out stackUpdateResponse
		_, res, err := doRequest(req, &out)
		assert.NoError(t, err)
		assert.Equal(t, "200 OK", res.Status, "should get a 201")
		assert.Empty(t, out.Error, "there is no error")
		assert.Equal(t, out.ID, "specific-id", "id has not changed")
		assert.Equal(t, out.Ok, true, "ok is true")
		assert.NotEmpty(t, out.Rev, "there is a rev")
		assert.Equal(t, out.ID, out.Data.ID(), "in doc id is simple uuid")
		assert.Equal(t, out.Type, out.Data.Type, "in doc type is correct")
		assert.Equal(t, out.Rev, out.Data.Rev(), "in doc rev is correct")
		assert.Equal(t, "anewvalue", out.Data.Get("somefield"), "content has changed")
	})

	t.Run("NoRevInDocUpdate", func(t *testing.T) {
		// Get revision
		doc := getDocForTest()
		// update it
		in := jsonReader(&map[string]interface{}{
			"_id":       doc.ID(),
			"test":      doc.M["test"],
			"somefield": "anewvalue",
		})
		url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID()
		req, _ := http.NewRequest("PUT", url, in)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		_, res, err := doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
	})

	t.Run("PreviousRevInDocUpdate", func(t *testing.T) {
		// Get revision
		doc := getDocForTest()
		firstRev := doc.Rev()
		url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID()

		// correcly update it
		in := jsonReader(&map[string]interface{}{
			"_id":       doc.ID(),
			"_rev":      doc.Rev(),
			"somefield": "anewvalue",
		})
		req, _ := http.NewRequest("PUT", url, in)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		_, res, err := doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, "200 OK", res.Status, "first update should work")

		// update it
		in2 := jsonReader(&map[string]interface{}{
			"_id":       doc.ID(),
			"_rev":      firstRev,
			"somefield": "anewvalue2",
		})
		req2, _ := http.NewRequest("PUT", url, in2)
		req2.Header.Add("Authorization", "Bearer "+token)
		req2.Header.Set("Content-Type", "application/json")
		_, res2, err := doRequest(req2, nil)
		assert.NoError(t, err)
		assert.Equal(t, "409 Conflict", res2.Status, "should get a 409")
	})

	t.Run("SuccessDeleteIfMatch", func(t *testing.T) {
		// Get revision
		doc := getDocForTest()
		rev := doc.Rev()

		// Do deletion
		url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID()
		req, _ := http.NewRequest("DELETE", url, nil)
		req.Header.Add("If-Match", rev)
		req.Header.Add("Authorization", "Bearer "+token)
		var out stackUpdateResponse
		_, res, err := doRequest(req, &out)
		assert.NoError(t, err)
		assert.Equal(t, "200 OK", res.Status, "should get a 201")
		assert.Equal(t, out.Ok, true, "ok at top level (couchdb compatibility)")
		assert.Equal(t, out.ID, doc.ID(), "id at top level (couchdb compatibility)")
		assert.Equal(t, out.Deleted, true, "id at top level (couchdb compatibility)")
		assert.NotEqual(t, out.Rev, doc.Rev(), "id at top level (couchdb compatibility)")
	})

	t.Run("FailDeleteIfNotMatch", func(t *testing.T) {
		// Get revision
		doc := getDocForTest()

		// Do deletion
		url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID()
		req, _ := http.NewRequest("DELETE", url, nil)
		req.Header.Add("If-Match", "1-238238232322121") // not correct rev
		req.Header.Add("Authorization", "Bearer "+token)
		var out stackUpdateResponse
		_, res, err := doRequest(req, &out)
		assert.NoError(t, err)
		assert.Equal(t, "409 Conflict", res.Status, "should get a 409")
	})

	t.Run("FailDeleteIfHeaderAndRevMismatch", func(t *testing.T) {
		// Get revision
		doc := getDocForTest()

		// Do deletion
		url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID() + "?rev=1-238238232322121"
		req, _ := http.NewRequest("DELETE", url, nil)
		req.Header.Add("If-Match", "1-23823823231") // not same rev
		req.Header.Add("Authorization", "Bearer "+token)
		var out stackUpdateResponse
		_, res, err := doRequest(req, &out)
		assert.NoError(t, err)
		assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
	})

	t.Run("FailDeleteIfNoRev", func(t *testing.T) {
		// Get revision
		doc := getDocForTest()

		// Do deletion
		url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID()
		req, _ := http.NewRequest("DELETE", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		var out stackUpdateResponse
		_, res, err := doRequest(req, &out)
		assert.NoError(t, err)
		assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
	})

	t.Run("DefineIndex", func(t *testing.T) {
		def := M{"index": M{"fields": S{"foo"}}}
		url := ts.URL + "/data/" + Type + "/_index"
		req, _ := http.NewRequest("POST", url, jsonReader(&def))
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		var out indexCreationResponse
		_, res, err := doRequest(req, &out)
		assert.NoError(t, err)
		assert.Equal(t, "200 OK", res.Status, "first update should work")
		assert.Empty(t, out.Error, "should have no error")
		assert.Empty(t, out.Reason, "should have no error")
		assert.Equal(t, "created", out.Result, "should have created result")
		assert.NotEmpty(t, out.Name, "should have a name")
		assert.NotEmpty(t, out.ID, "should have an design doc ID")
	})

	t.Run("ReDefineIndex", func(t *testing.T) {
		def := M{"index": M{"fields": S{"foo"}}}
		url := ts.URL + "/data/" + Type + "/_index"
		req, _ := http.NewRequest("POST", url, jsonReader(&def))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Add("Authorization", "Bearer "+token)
		var out indexCreationResponse
		_, res, err := doRequest(req, &out)
		assert.NoError(t, err)
		assert.Equal(t, "200 OK", res.Status)
		assert.Empty(t, out.Error, "should have no error")
		assert.Empty(t, out.Reason, "should have no error")
		assert.Equal(t, "exists", out.Result, "should have exists result")
		assert.NotEmpty(t, out.Name, "should have a name")
		assert.NotEmpty(t, out.ID, "should have an design doc ID")
	})

	t.Run("DefineIndexUnexistingDoctype", func(t *testing.T) {
		_ = couchdb.DeleteDB(testInstance, "io.cozy.nottype")

		def := M{"index": M{"fields": S{"foo"}}}
		url := ts.URL + "/data/io.cozy.nottype/_index"
		req, _ := http.NewRequest("POST", url, jsonReader(&def))
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		var out indexCreationResponse
		_, res, err := doRequest(req, &out)
		assert.NoError(t, err)
		assert.Equal(t, "200 OK", res.Status)
		assert.Empty(t, out.Error, "should have no error")
		assert.Empty(t, out.Reason, "should have no error")
		assert.Equal(t, "created", out.Result, "should have created result")
		assert.NotEmpty(t, out.Name, "should have a name")
		assert.NotEmpty(t, out.ID, "should have an design doc ID")
	})

	t.Run("FindDocuments", func(t *testing.T) {
		_ = couchdb.ResetDB(testInstance, Type)

		_ = getDocForTest()
		_ = getDocForTest()
		_ = getDocForTest()

		def := M{"index": M{"fields": S{"test"}}}
		url := ts.URL + "/data/" + Type + "/_index"
		req, _ := http.NewRequest("POST", url, jsonReader(&def))
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		var out indexCreationResponse
		_, _, err := doRequest(req, &out)
		assert.NoError(t, err)
		assert.Empty(t, out.Error, "should have no error")

		query := M{"selector": M{"test": "value"}}
		url2 := ts.URL + "/data/" + Type + "/_find"
		req, _ = http.NewRequest("POST", url2, jsonReader(&query))
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		var out2 struct {
			Docs           []couchdb.JSONDoc       `json:"docs"`
			ExecutionStats *couchdb.ExecutionStats `json:"execution_stats,omitempty"`
		}
		_, res, err := doRequest(req, &out2)
		assert.Equal(t, "200 OK", res.Status, "should get a 200")
		assert.NoError(t, err)
		assert.Len(t, out2.Docs, 3, "should have found 3 docs")
		assert.Nil(t, out2.ExecutionStats)
	})

	t.Run("FindDocumentsWithStats", func(t *testing.T) {
		_ = couchdb.ResetDB(testInstance, Type)

		_ = getDocForTest()

		def := M{"index": M{"fields": S{"test"}}}
		url := ts.URL + "/data/" + Type + "/_index"
		req, _ := http.NewRequest("POST", url, jsonReader(&def))
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		var out indexCreationResponse
		_, _, err := doRequest(req, &out)
		assert.NoError(t, err)
		assert.Empty(t, out.Error, "should have no error")

		query := M{"selector": M{"test": "value"}, "execution_stats": true}
		url2 := ts.URL + "/data/" + Type + "/_find"
		req, _ = http.NewRequest("POST", url2, jsonReader(&query))
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		var out2 struct {
			Docs           []couchdb.JSONDoc       `json:"docs"`
			ExecutionStats *couchdb.ExecutionStats `json:"execution_stats,omitempty"`
		}
		_, res, err := doRequest(req, &out2)
		assert.Equal(t, "200 OK", res.Status, "should get a 200")
		assert.NoError(t, err)
		assert.NotEmpty(t, out2.ExecutionStats)
	})

	t.Run("FindDocumentsPaginated", func(t *testing.T) {
		_ = couchdb.ResetDB(testInstance, Type)

		for i := 1; i <= 150; i++ {
			_ = getDocForTest()
		}

		def := M{"index": M{"fields": S{"test"}}}
		url := ts.URL + "/data/" + Type + "/_index"
		req, _ := http.NewRequest("POST", url, jsonReader(&def))
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		var out indexCreationResponse
		_, _, err := doRequest(req, &out)
		assert.NoError(t, err)
		assert.Empty(t, out.Error, "should have no error")

		query := M{"selector": M{"test": "value"}}
		url2 := ts.URL + "/data/" + Type + "/_find"
		req, _ = http.NewRequest("POST", url2, jsonReader(&query))
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		var out2 struct {
			Limit int
			Next  bool
			Docs  []couchdb.JSONDoc `json:"docs"`
		}
		_, res, err := doRequest(req, &out2)
		assert.Equal(t, "200 OK", res.Status, "should get a 200")
		assert.NoError(t, err)
		assert.Len(t, out2.Docs, 100, "should stop at 100 docs")
		assert.Equal(t, 100, out2.Limit)
		assert.Equal(t, true, out2.Next)

		query2 := M{"selector": M{"test": "value"}, "limit": 10}
		req, _ = http.NewRequest("POST", url2, jsonReader(&query2))
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		var out3 struct {
			Limit int
			Next  bool
			Docs  []couchdb.JSONDoc `json:"docs"`
		}
		_, res, err = doRequest(req, &out3)
		assert.Equal(t, "200 OK", res.Status, "should get a 200")
		assert.NoError(t, err)
		// assert.Len(t, out3.Docs, 10, "should stop at 100 docs")
		assert.Equal(t, 10, out3.Limit)
		assert.Equal(t, true, out3.Next)
	})

	t.Run("FindDocumentsPaginatedBookmark", func(t *testing.T) {
		_ = couchdb.ResetDB(testInstance, Type)

		for i := 1; i <= 200; i++ {
			_ = getDocForTest()
		}

		def := M{"index": M{"fields": S{"test"}}}
		url := ts.URL + "/data/" + Type + "/_index"
		req, _ := http.NewRequest("POST", url, jsonReader(&def))
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		var out indexCreationResponse
		_, _, err := doRequest(req, &out)
		assert.NoError(t, err)
		assert.Empty(t, out.Error, "should have no error")

		query := M{"selector": M{"test": "value"}}
		url2 := ts.URL + "/data/" + Type + "/_find"
		req, _ = http.NewRequest("POST", url2, jsonReader(&query))
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		var out2 struct {
			Limit    int
			Next     bool
			Docs     []couchdb.JSONDoc `json:"docs"`
			Bookmark string
		}
		_, res, err := doRequest(req, &out2)
		assert.Equal(t, 200, res.StatusCode)
		assert.NoError(t, err)
		assert.Len(t, out2.Docs, 100, "should stop at 100 docs")
		assert.Equal(t, 100, out2.Limit)
		assert.Equal(t, true, out2.Next)
		assert.NotEmpty(t, out2.Bookmark)

		query2 := M{"selector": M{"test": "value"}, "bookmark": out2.Bookmark}
		req, _ = http.NewRequest("POST", url2, jsonReader(&query2))
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		_, res, err = doRequest(req, &out2)
		assert.Equal(t, 200, res.StatusCode)
		assert.NoError(t, err)
		assert.Len(t, out2.Docs, 100)
		assert.Equal(t, 100, out2.Limit)
		assert.Equal(t, true, out2.Next)

		query3 := M{"selector": M{"test": "value"}, "bookmark": out2.Bookmark}
		req, _ = http.NewRequest("POST", url2, jsonReader(&query3))
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		_, res, err = doRequest(req, &out2)
		assert.Equal(t, 200, res.StatusCode)
		assert.NoError(t, err)
		assert.Len(t, out2.Docs, 0)
		assert.Equal(t, false, out2.Next)

		var query4 = M{"selector": M{"test": "novalue"}}
		req, _ = http.NewRequest("POST", url2, jsonReader(&query4))
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		_, res, err = doRequest(req, &out2)
		assert.NoError(t, err)
		assert.Equal(t, 200, res.StatusCode)
		assert.Len(t, out2.Docs, 0)
		assert.Equal(t, "", out2.Bookmark)
	})

	t.Run("FindDocumentsWithoutIndex", func(t *testing.T) {
		query := M{"selector": M{"no-index-for-this-field": "value"}}
		url2 := ts.URL + "/data/" + Type + "/_find"
		req, _ := http.NewRequest("POST", url2, jsonReader(&query))
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		var out2 struct {
			Error  string `json:"error"`
			Reason string `json:"reason"`
		}
		_, res, err := doRequest(req, &out2)
		assert.Equal(t, "400 Bad Request", res.Status, "should get a 200")
		assert.NoError(t, err)
		assert.Contains(t, out2.Error, "no_index")
		assert.Contains(t, out2.Reason, "no matching index")
	})

	t.Run("GetChanges", func(t *testing.T) {
		assert.NoError(t, couchdb.ResetDB(testInstance, Type))

		url := ts.URL + "/data/" + Type + "/_changes?style=all_docs"
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		out, res, err := doRequest(req, nil)
		assert.Equal(t, "200 OK", res.Status, "should get a 200")
		assert.NoError(t, err)
		seqno := out["last_seq"].(string)

		// creates 3 docs
		_ = getDocForTest()
		_ = getDocForTest()
		_ = getDocForTest()

		url = ts.URL + "/data/" + Type + "/_changes?limit=2&since=" + seqno
		req, _ = http.NewRequest("GET", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		out, res, err = doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, 200, res.StatusCode)
		assert.Len(t, out["results"].([]interface{}), 2)

		url = ts.URL + "/data/" + Type + "/_changes?since=" + seqno
		req, _ = http.NewRequest("GET", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		out, res, err = doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, 200, res.StatusCode)
		assert.Len(t, out["results"].([]interface{}), 3)
	})

	t.Run("PostChanges", func(t *testing.T) {
		assert.NoError(t, couchdb.ResetDB(testInstance, Type))

		// creates 3 docs
		doc1 := getDocForTest()
		_ = getDocForTest()
		doc2 := getDocForTest()

		payload := fmt.Sprintf(`{"doc_ids": ["%s", "%s"]}`, doc1.ID(), doc2.ID())

		url := ts.URL + "/data/" + Type + "/_changes?include_docs=true&filter=_doc_ids"
		req, _ := http.NewRequest("POST", url, bytes.NewReader([]byte(payload)))
		req.Header.Add("Authorization", "Bearer "+token)
		out, res, err := doRequest(req, nil)
		assert.Equal(t, 200, res.StatusCode, "should get a 200")
		assert.NoError(t, err)
		assert.Len(t, out["results"].([]interface{}), 2)
	})

	t.Run("WrongFeedChanges", func(t *testing.T) {
		url := ts.URL + "/data/" + Type + "/_changes?feed=continuous"
		req, _ := http.NewRequest("POST", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		_, res, err := doRequest(req, nil)
		assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
		assert.NoError(t, err)
	})

	t.Run("WrongStyleChanges", func(t *testing.T) {
		url := ts.URL + "/data/" + Type + "/_changes?style=not_a_valid_style"
		req, _ := http.NewRequest("POST", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		_, res, err := doRequest(req, nil)
		assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
		assert.NoError(t, err)
	})

	t.Run("LimitIsNoNumber", func(t *testing.T) {
		url := ts.URL + "/data/" + Type + "/_changes?limit=not_a_number"
		req, _ := http.NewRequest("POST", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		_, res, err := doRequest(req, nil)
		assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
		assert.NoError(t, err)
	})

	t.Run("UnsupportedOption", func(t *testing.T) {
		url := ts.URL + "/data/" + Type + "/_changes?inlude_docs=true"
		req, _ := http.NewRequest("POST", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		_, res, err := doRequest(req, nil)
		assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
		assert.NoError(t, err)
	})

	t.Run("GetAllDocs", func(t *testing.T) {
		url := ts.URL + "/data/" + Type + "/_all_docs?include_docs=true"
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		out, res, err := doRequest(req, nil)
		assert.Equal(t, "200 OK", res.Status, "should get a 200")
		assert.NoError(t, err)
		totalRows := out["total_rows"].(float64)
		assert.Equal(t, float64(3), totalRows)
		offset := out["offset"].(float64)
		assert.Equal(t, float64(0), offset)
		rows := out["rows"].([]interface{})
		assert.Len(t, rows, 3)
		row := rows[0].(map[string]interface{})
		id := row["id"].(string)
		assert.NotEmpty(t, id)
		doc, ok := row["doc"].(map[string]interface{})
		assert.True(t, ok)
		value := doc["test"].(string)
		assert.Equal(t, "value", value)
		foo := doc["foo"].(map[string]interface{})
		bar := foo["bar"].(string)
		assert.Equal(t, "one", bar)
		baz := foo["baz"].(string)
		assert.Equal(t, "two", baz)
		qux := foo["qux"].(string)
		assert.Equal(t, "quux", qux)
	})

	t.Run("GetAllDocsWithFields", func(t *testing.T) {
		url := ts.URL + "/data/" + Type + "/_all_docs?include_docs=true&Fields=test,nosuchfield,foo.qux"
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		out, res, err := doRequest(req, nil)
		assert.Equal(t, "200 OK", res.Status, "should get a 200")
		require.NoError(t, err)
		totalRows := out["total_rows"].(float64)
		assert.Equal(t, float64(3), totalRows)
		offset := out["offset"].(float64)
		assert.Equal(t, float64(0), offset)
		rows := out["rows"].([]interface{})
		assert.Len(t, rows, 3)
		row := rows[0].(map[string]interface{})
		id := row["id"].(string)
		assert.NotEmpty(t, id)
		doc, ok := row["doc"].(map[string]interface{})
		assert.True(t, ok)
		value := doc["test"].(string)
		assert.Equal(t, "value", value)
		foo := doc["foo"].(map[string]interface{})
		assert.NotContains(t, foo, "bar")
		assert.NotContains(t, foo, "baz")
		qux := foo["qux"].(string)
		assert.Equal(t, "quux", qux)
		assert.NotContains(t, doc, "courge")
	})

	t.Run("NormalDocs", func(t *testing.T) {
		view := &couchdb.View{
			Name:    "foobar",
			Doctype: Type,
			Map: `
  function(doc) {
    emit(doc.foobar, doc);
  }`,
		}
		g, _ := errgroup.WithContext(context.Background())
		couchdb.DefineViews(g, testInstance, []*couchdb.View{view})
		assert.NoError(t, g.Wait())

		err := couchdb.CreateNamedDoc(testInstance, &couchdb.JSONDoc{
			Type: Type,
			M: map[string]interface{}{
				"_id":  "four",
				"test": "fourthvalue",
			},
		})
		assert.NoError(t, err)

		url := ts.URL + "/data/" + Type + "/_normal_docs?limit=2"
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		out, res, err := doRequest(req, nil)
		assert.Equal(t, "200 OK", res.Status, "should get a 200")
		assert.NoError(t, err)
		totalRows := out["total_rows"].(float64)
		assert.Equal(t, float64(4), totalRows)
		rows := out["rows"].([]interface{})
		assert.Len(t, rows, 2)
		row := rows[0].(map[string]interface{})
		id := row["_id"].(string)
		assert.NotEmpty(t, id)
		value := row["test"].(string)
		assert.Equal(t, "value", value)
		bookmark := out["bookmark"].(string)
		assert.NotEmpty(t, bookmark)
		executionStats := out["execution_stats"]
		assert.Nil(t, executionStats)

		// skip pagination
		url = ts.URL + "/data/" + Type + "/_normal_docs?limit=2&skip=2"
		req, _ = http.NewRequest("GET", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		out, res, err = doRequest(req, nil)
		assert.Equal(t, "200 OK", res.Status, "should get a 200")
		assert.NoError(t, err)
		totalRows = out["total_rows"].(float64)
		assert.Equal(t, float64(4), totalRows)
		rows = out["rows"].([]interface{})
		assert.Len(t, rows, 2)
		row = rows[1].(map[string]interface{})
		id = row["_id"].(string)
		assert.NotEmpty(t, id)
		value = row["test"].(string)
		assert.Equal(t, "fourthvalue", value)

		// bookmark pagination
		url = ts.URL + "/data/" + Type + "/_normal_docs?bookmark=" + bookmark
		req, _ = http.NewRequest("GET", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		out, res, err = doRequest(req, nil)
		assert.Equal(t, "200 OK", res.Status, "should get a 200")
		assert.NoError(t, err)
		totalRows = out["total_rows"].(float64)
		assert.Equal(t, float64(4), totalRows)
		rows = out["rows"].([]interface{})
		assert.Len(t, rows, 2)
		row = rows[1].(map[string]interface{})
		id = row["_id"].(string)
		assert.NotEmpty(t, id)
		value = row["test"].(string)
		assert.Equal(t, "fourthvalue", value)

		emptyType := "io.cozy.anothertype"
		_ = couchdb.ResetDB(testInstance, emptyType)
		url = ts.URL + "/data/" + emptyType + "/_normal_docs"
		req, _ = http.NewRequest("GET", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		out, res, err = doRequest(req, nil)
		assert.Equal(t, "200 OK", res.Status, "should get a 200")
		assert.NoError(t, err)
		totalRows = out["total_rows"].(float64)
		assert.Equal(t, float64(0), totalRows)
		bookmark = out["bookmark"].(string)
		assert.Equal(t, "", bookmark)

		// execution stats
		url = ts.URL + "/data/" + emptyType + "/_normal_docs?execution_stats=true"
		req, _ = http.NewRequest("GET", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		out, res, err = doRequest(req, nil)
		assert.Equal(t, "200 OK", res.Status, "should get a 200")
		assert.NoError(t, err)
		assert.NotEmpty(t, out["execution_stats"])
	})

	t.Run("GetDesignDocs", func(t *testing.T) {
		def := M{"index": M{"fields": S{"foo"}}}
		_, err := couchdb.DefineIndexRaw(testInstance, Type, &def)
		assert.NoError(t, err)

		url := ts.URL + "/data/" + Type + "/_design_docs"
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		out, res, err := doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, "200 OK", res.Status)
		rows := out["rows"].([]interface{})
		nDesignDocs := len(rows)
		assert.Greater(t, nDesignDocs, 0)
		firstRow := rows[0].(map[string]interface{})
		id := firstRow["id"].(string)
		rev := firstRow["value"].(map[string]interface{})["rev"].(string)
		assert.NotEmpty(t, id)
		assert.NotEmpty(t, rev)
	})

	t.Run("GetDesignDoc", func(t *testing.T) {
		ddoc := "myindex"
		def := M{"index": M{"fields": S{"foo"}}, "ddoc": ddoc}
		_, err := couchdb.DefineIndexRaw(testInstance, Type, &def)
		assert.NoError(t, err)

		url := ts.URL + "/data/" + Type + "/_design/" + ddoc
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		out, res, err := doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, "200 OK", res.Status)

		id := out["_id"].(string)
		rev := out["_rev"].(string)

		assert.Equal(t, "_design/"+ddoc, id)
		assert.NotEmpty(t, rev)
	})

	t.Run("DeleteDesignDoc", func(t *testing.T) {
		def := M{"index": M{"fields": S{"foo"}}}
		_, err := couchdb.DefineIndexRaw(testInstance, Type, &def)
		assert.NoError(t, err)

		url := ts.URL + "/data/" + Type + "/_design_docs"
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		out, res, err := doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, "200 OK", res.Status)
		rows := out["rows"].([]interface{})
		nDesignDocs := len(rows)
		assert.Greater(t, nDesignDocs, 0)
		firstRow := rows[0].(map[string]interface{})
		id := firstRow["id"].(string)
		ddoc := strings.Split(id, "/")[1]
		rev := firstRow["value"].(map[string]interface{})["rev"].(string)

		url = ts.URL + "/data/" + Type + "/_design/" + ddoc + "?rev=" + rev
		req, _ = http.NewRequest("DELETE", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		_, res, err = doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, "200 OK", res.Status)

		url = ts.URL + "/data/" + Type + "/_design_docs"
		req, _ = http.NewRequest("GET", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		out, res, err = doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, "200 OK", res.Status)

		rows = out["rows"].([]interface{})
		assert.Less(t, len(rows), nDesignDocs)
	})

	t.Run("CannotDeleteStackDesignDoc", func(t *testing.T) {
		url := ts.URL + "/data/io.cozy.files/_design_docs"
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		out, res, err := doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, 200, res.StatusCode)
		rows := out["rows"].([]interface{})
		var indexRev, viewRev string
		for _, row := range rows {
			info := row.(map[string]interface{})
			if info["id"] == "_design/dir-by-path" {
				value := info["value"].(map[string]interface{})
				indexRev = value["rev"].(string)
			}
			if info["id"] == "_design/by-parent-type-name" {
				value := info["value"].(map[string]interface{})
				viewRev = value["rev"].(string)
			}
		}

		url = ts.URL + "/data/io.cozy.files/_design/dir-by-path?rev=" + indexRev
		req, _ = http.NewRequest("DELETE", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		_, res, err = doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, 403, res.StatusCode)

		url = ts.URL + "/data/io.cozy.files/_design/by-parent-type-name?rev=" + viewRev
		req, _ = http.NewRequest("DELETE", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		_, res, err = doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, 403, res.StatusCode)
	})

	t.Run("CopyDesignDoc", func(t *testing.T) {
		srcDdoc := "indextocopy"
		targetID := "_design/indexcopied"
		def := M{"index": M{"fields": S{"foo"}}, "ddoc": srcDdoc}
		_, err := couchdb.DefineIndexRaw(testInstance, Type, &def)
		assert.NoError(t, err)

		url := ts.URL + "/data/" + Type + "/_design/" + srcDdoc
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		out, res, err := doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, "200 OK", res.Status)

		rev := out["_rev"].(string)

		url = ts.URL + "/data/" + Type + "/_design/" + srcDdoc + "/copy?rev=" + rev
		req, _ = http.NewRequest("POST", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Destination", targetID)
		out, res, err = doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, "201 Created", res.Status)

		assert.Equal(t, targetID, out["id"].(string))
		assert.Equal(t, rev, out["rev"].(string))
	})

	t.Run("DeleteDatabase", func(t *testing.T) {
		url := ts.URL + "/data/" + Type + "/"
		req, _ := http.NewRequest("DELETE", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		out, res, err := doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, "200 OK", res.Status)
		assert.Equal(t, true, out["deleted"].(bool))
	})

	t.Run("DeleteDatabaseNoPermission", func(t *testing.T) {
		doctype := "io.cozy.forbidden"
		url := ts.URL + "/data/" + doctype + "/"
		req, _ := http.NewRequest("DELETE", url, nil)
		req.Header.Add("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		_, res, err := doRequest(req, nil)
		assert.NoError(t, err)
		assert.Equal(t, "403 Forbidden", res.Status)
	})

}

func jsonReader(data interface{}) io.Reader {
	bs, _ := json.Marshal(&data)
	return bytes.NewReader(bs)
}

func doRequest(req *http.Request, out interface{}) (jsonres map[string]interface{}, res *http.Response, err error) {
	res, err = client.Do(req)
	if err != nil {
		return
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return
	}
	if out == nil {
		var out map[string]interface{}
		err = json.Unmarshal(body, &out)
		if err != nil {
			return
		}
		return out, res, err
	}
	err = json.Unmarshal(body, &out)
	if err != nil {
		return
	}
	return nil, res, err
}

func getDocForTest() *couchdb.JSONDoc {
	doc := couchdb.JSONDoc{
		Type: Type,
		M: map[string]interface{}{
			"test":   "value",
			"foo":    map[string]interface{}{"bar": "one", "baz": "two", "qux": "quux"},
			"courge": 1,
		},
	}
	_ = couchdb.CreateDoc(testInstance, &doc)
	return &doc
}
