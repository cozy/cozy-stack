package data

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
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
	body, err := ioutil.ReadAll(res.Body)
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

func getDocForTest() couchdb.JSONDoc {
	doc := couchdb.JSONDoc{Type: Type, M: map[string]interface{}{"test": "value"}}
	_ = couchdb.CreateDoc(testInstance, &doc)
	return doc
}

func TestMain(m *testing.M) {
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

	os.Exit(setup.Run())
}

func TestSuccessGet(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/data/"+Type+"/"+ID, nil)
	req.Header.Add("Authorization", "Bearer "+token)
	out, res, err := doRequest(req, nil)
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status, "should get a 200")
	if assert.Contains(t, out, "test") {
		assert.Equal(t, out["test"], "testvalue", "should give the same doc")
	}
}

func TestGetWithSlash(t *testing.T) {
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
}

func TestWrongDoctype(t *testing.T) {
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

}

func TestUnderscoreName(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/data/"+Type+"/_foo", nil)
	req.Header.Add("Authorization", "Bearer "+token)
	_, res, err := doRequest(req, nil)
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
}

func TestGetDesign(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/data/"+Type+"/_design/test", nil)
	req.Header.Add("Authorization", "Bearer "+token)
	_, res, err := doRequest(req, nil)
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
}

func TestVFSDoctype(t *testing.T) {

	var in = jsonReader(&map[string]interface{}{
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
}

func TestWrongID(t *testing.T) {
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
}

func TestWrongHost(t *testing.T) {
	t.Skip("unskip me when we stop falling back to Host = dev")
	req, _ := http.NewRequest("GET", ts.URL+"/data/"+Type+"/"+ID, nil)
	out, res, err := doRequest(req, nil)
	assert.NoError(t, err)
	assert.Equal(t, "404 Not Found", res.Status, "should get a 404")
	if assert.Contains(t, out, "error") {
		assert.Equal(t, "not_found", out["error"], "should give a json name")
	}
	if assert.Contains(t, out, "reason") {
		assert.Equal(t, "wrong_doctype", out["reason"], "should give a reason")
	}
}

func TestSuccessCreateKnownDoctype(t *testing.T) {
	var in = jsonReader(&map[string]interface{}{
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
}

func TestSuccessCreateUnknownDoctype(t *testing.T) {
	var in = jsonReader(&map[string]interface{}{
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
}

func TestWrongCreateWithID(t *testing.T) {
	var in = jsonReader(&map[string]interface{}{
		"_id":       "this-should-not-be-an-id",
		"somefield": "avalue",
	})
	req, _ := http.NewRequest("POST", ts.URL+"/data/"+Type+"/", in)
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	_, res, err := doRequest(req, nil)
	assert.NoError(t, err)
	assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
}

func TestSuccessUpdate(t *testing.T) {

	// Get revision
	doc := getDocForTest()
	url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID()

	// update it
	var in = jsonReader(&map[string]interface{}{
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
}

// Test for having not the same ID in document and URL
func TestWrongIDInDocUpdate(t *testing.T) {
	// Get revision
	doc := getDocForTest()
	// update it
	var in = jsonReader(&map[string]interface{}{
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
}

// Test for having an inexisting id at all
func TestCreateDocWithAFixedID(t *testing.T) {
	// update it
	var in = jsonReader(&map[string]interface{}{
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

}

func TestNoRevInDocUpdate(t *testing.T) {
	// Get revision
	doc := getDocForTest()
	// update it
	var in = jsonReader(&map[string]interface{}{
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
}

func TestPreviousRevInDocUpdate(t *testing.T) {
	// Get revision
	doc := getDocForTest()
	firstRev := doc.Rev()
	url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID()

	// correcly update it
	var in = jsonReader(&map[string]interface{}{
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
	var in2 = jsonReader(&map[string]interface{}{
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
}

func TestSuccessDeleteIfMatch(t *testing.T) {
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
}

func TestFailDeleteIfNotMatch(t *testing.T) {
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
}

func TestFailDeleteIfHeaderAndRevMismatch(t *testing.T) {
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
}

func TestFailDeleteIfNoRev(t *testing.T) {
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
}

type M map[string]interface{}
type S []interface{}
type indexCreationResponse struct {
	Result string `json:"result"`
	Error  string `json:"error"`
	Reason string `json:"reason"`
	ID     string `json:"id"`
	Name   string `json:"name"`
}

func TestDefineIndex(t *testing.T) {
	var def = M{"index": M{"fields": S{"foo"}}}
	var url = ts.URL + "/data/" + Type + "/_index"
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
}

func TestReDefineIndex(t *testing.T) {
	var def = M{"index": M{"fields": S{"foo"}}}
	var url = ts.URL + "/data/" + Type + "/_index"
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
}

func TestDefineIndexUnexistingDoctype(t *testing.T) {
	_ = couchdb.DeleteDB(testInstance, "io.cozy.nottype")

	var def = M{"index": M{"fields": S{"foo"}}}
	var url = ts.URL + "/data/io.cozy.nottype/_index"
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
}

func TestFindDocuments(t *testing.T) {
	_ = couchdb.ResetDB(testInstance, Type)

	_ = getDocForTest()
	_ = getDocForTest()
	_ = getDocForTest()

	var def = M{"index": M{"fields": S{"test"}}}
	var url = ts.URL + "/data/" + Type + "/_index"
	req, _ := http.NewRequest("POST", url, jsonReader(&def))
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	var out indexCreationResponse
	_, _, err := doRequest(req, &out)
	assert.NoError(t, err)
	assert.Empty(t, out.Error, "should have no error")

	var query = M{"selector": M{"test": "value"}}
	var url2 = ts.URL + "/data/" + Type + "/_find"
	req, _ = http.NewRequest("POST", url2, jsonReader(&query))
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	var out2 struct {
		Docs []couchdb.JSONDoc `json:"docs"`
	}
	_, res, err := doRequest(req, &out2)
	assert.Equal(t, "200 OK", res.Status, "should get a 200")
	assert.NoError(t, err)
	assert.Len(t, out2.Docs, 3, "should have found 3 docs")
}

func TestFindDocumentsPaginated(t *testing.T) {
	_ = couchdb.ResetDB(testInstance, Type)

	for i := 1; i <= 150; i++ {
		_ = getDocForTest()
	}

	var def = M{"index": M{"fields": S{"test"}}}
	var url = ts.URL + "/data/" + Type + "/_index"
	req, _ := http.NewRequest("POST", url, jsonReader(&def))
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	var out indexCreationResponse
	_, _, err := doRequest(req, &out)
	assert.NoError(t, err)
	assert.Empty(t, out.Error, "should have no error")

	var query = M{"selector": M{"test": "value"}}
	var url2 = ts.URL + "/data/" + Type + "/_find"
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

	var query2 = M{"selector": M{"test": "value"}, "limit": 10}
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
}

func TestFindDocumentsWithoutIndex(t *testing.T) {
	var query = M{"selector": M{"no-index-for-this-field": "value"}}
	var url2 = ts.URL + "/data/" + Type + "/_find"
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
}

func TestGetChanges(t *testing.T) {

	assert.NoError(t, couchdb.ResetDB(testInstance, Type))

	url := ts.URL + "/data/" + Type + "/_changes?style=all_docs"
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("Authorization", "Bearer "+token)
	out, res, err := doRequest(req, nil)
	assert.Equal(t, "200 OK", res.Status, "should get a 200")
	assert.NoError(t, err)
	var seqno = out["last_seq"].(string)

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
}

func TestPostChanges(t *testing.T) {
	url := ts.URL + "/data/" + Type + "/_changes?style=all_docs"
	req, _ := http.NewRequest("POST", url, nil)
	req.Header.Add("Authorization", "Bearer "+token)
	_, res, err := doRequest(req, nil)
	assert.Equal(t, "200 OK", res.Status, "should get a 200")
	assert.NoError(t, err)
}

func TestWrongFeedChanges(t *testing.T) {
	url := ts.URL + "/data/" + Type + "/_changes?feed=continuous"
	req, _ := http.NewRequest("POST", url, nil)
	req.Header.Add("Authorization", "Bearer "+token)
	_, res, err := doRequest(req, nil)
	assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
	assert.NoError(t, err)
}

func TestWrongStyleChanges(t *testing.T) {
	url := ts.URL + "/data/" + Type + "/_changes?style=not_a_valid_style"
	req, _ := http.NewRequest("POST", url, nil)
	req.Header.Add("Authorization", "Bearer "+token)
	_, res, err := doRequest(req, nil)
	assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
	assert.NoError(t, err)
}

func TestLimitIsNoNumber(t *testing.T) {
	url := ts.URL + "/data/" + Type + "/_changes?limit=not_a_number"
	req, _ := http.NewRequest("POST", url, nil)
	req.Header.Add("Authorization", "Bearer "+token)
	_, res, err := doRequest(req, nil)
	assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
	assert.NoError(t, err)
}

func TestUnsupportedOption(t *testing.T) {
	url := ts.URL + "/data/" + Type + "/_changes?inlude_docs=true"
	req, _ := http.NewRequest("POST", url, nil)
	req.Header.Add("Authorization", "Bearer "+token)
	_, res, err := doRequest(req, nil)
	assert.Equal(t, "400 Bad Request", res.Status, "should get a 400")
	assert.NoError(t, err)
}

func TestAccountsEncryptDecrypt(t *testing.T) {
	config.UseTestFile()

	v := []byte(`
{
    "_id": "d01aa821781612dce542a13d6989e6d0",
    "_rev": "5-c8fc2169ff3226165688865e7cb609ef",
    "_type": "io.cozy.accounts",
    "account_type": "labanquepostale44",
    "auth": {
        "accountName": "Perso",
        "identifier": "WHATYOUWANT",
        "secret": "YOUWANTTOREADMYSECRET"
    },
    "data": {
        "account_type": "linxo",
        "auth": {
            "login": "linxo.SOMEID@cozy.rocks",
            "password": "SOMEPASSWORD"
        },
        "status": "connected",
        "token": "4D757B74AD",
        "uuid": "f6bb19cf-1c03-4d80-92e9-af66c18c4aa4"
    },
    "type": "io.cozy.accounts"
}
`)

	var encrypted, decrypted bool
	var m1 map[string]interface{} // original
	var m2 map[string]interface{} // encrypted
	var m3 map[string]interface{} // decrypted
	assert.NoError(t, json.Unmarshal(v, &m1))
	assert.NoError(t, json.Unmarshal(v, &m2))
	assert.NoError(t, json.Unmarshal(v, &m3))

	encrypted = encryptMap(m2)
	assert.True(t, encrypted)

	{
		auth1 := m2["auth"].(map[string]interface{})
		auth2 := m2["data"].(map[string]interface{})["auth"].(map[string]interface{})
		{
			_, ok1 := auth1["secret"]
			_, ok2 := auth1["secret_encrypted"]
			assert.False(t, ok1)
			assert.True(t, ok2)
		}
		{
			_, ok1 := auth2["password"]
			_, ok2 := auth2["credentials_encrypted"]
			assert.False(t, ok1)
			assert.True(t, ok2)
		}
	}

	encrypted = encryptMap(m3)
	decrypted = decryptMap(m3)
	assert.True(t, encrypted)
	assert.True(t, decrypted)
	assert.EqualValues(t, m1, m3)

	{
		auth1 := m3["auth"].(map[string]interface{})
		auth2 := m3["data"].(map[string]interface{})["auth"].(map[string]interface{})
		{
			_, ok1 := auth1["secret"]
			_, ok2 := auth1["secret_encrypted"]
			assert.True(t, ok1)
			assert.False(t, ok2)
		}
		{
			_, ok1 := auth2["password"]
			_, ok2 := auth2["credentials_encrypted"]
			assert.True(t, ok1)
			assert.False(t, ok2)
		}
	}
}

func TestGetAllDocs(t *testing.T) {
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
}

func TestNormalDocs(t *testing.T) {
	view := &couchdb.View{
		Name:    "foobar",
		Doctype: Type,
		Map: `
function(doc) {
  emit(doc.foobar, doc);
}
		`,
	}
	err := couchdb.DefineViews(testInstance, []*couchdb.View{view})
	assert.NoError(t, err)

	err = couchdb.CreateNamedDoc(testInstance, &couchdb.JSONDoc{
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
}
