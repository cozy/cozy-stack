package data

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
	"github.com/sourcegraph/checkup"
	"github.com/stretchr/testify/assert"
)

var client = &http.Client{}

const COUCHURL = "http://localhost:5984/"
const HOST = "example.com"
const TYPE = "io.cozy.events"
const ID = "4521C325F6478E45"
const EXPECTEDDBNAME = "example-com%2Fio-cozy-events"

var DOCUMENT = []byte(`{
	"test": "testvalue"
}`)

var ts *httptest.Server

// @TODO this should be moved to our couchdb package or to
// some test helpers files.

func couchReq(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, COUCHURL+path, body)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return res, err
	}
	defer res.Body.Close()
	return res, nil
}

func jsonReader(data *map[string]interface{}) io.Reader {
	bs, _ := json.Marshal(&data)
	return bytes.NewReader(bs)
}

func doRequest(req *http.Request) (jsonres map[string]interface{}, res *http.Response, err error) {

	res, err = client.Do(req)
	if err != nil {
		return
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}
	var out map[string]interface{}
	err = json.Unmarshal(body, &out)
	if err != nil {
		return
	}
	return out, res, err
}

func injectInstance(instance *middlewares.Instance) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("instance", instance)
	}
}

func TestMain(m *testing.M) {

	// First we make sure couchdb is started
	couchdb, err := checkup.HTTPChecker{URL: COUCHURL}.Check()
	if err != nil || couchdb.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}

	router := gin.New()
	instance := &middlewares.Instance{
		Domain:     HOST,
		StorageURL: "mem://test",
	}
	router.Use(errors.Handler())
	router.Use(injectInstance(instance))
	Routes(router.Group("/data"))
	ts = httptest.NewServer(router)
	couchReq("DELETE", EXPECTEDDBNAME, nil)
	couchReq("PUT", EXPECTEDDBNAME, nil)
	couchReq("PUT", EXPECTEDDBNAME+"/"+TYPE+"%2F"+ID, bytes.NewReader(DOCUMENT))

	defer ts.Close()
	os.Exit(m.Run())
}

func TestSuccessGet(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/data/"+TYPE+"/"+ID, nil)
	req.Header.Add("Host", HOST)
	out, res, err := doRequest(req)
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status, "should get a 200")
	if assert.Contains(t, out, "test") {
		assert.Equal(t, out["test"], "testvalue", "should give the same doc")
	}
}

func TestWrongDoctype(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/data/nottype/"+ID, nil)
	req.Header.Add("Host", HOST)
	out, res, err := doRequest(req)
	assert.NoError(t, err)
	assert.Equal(t, "404 Not Found", res.Status, "should get a 404")
	if assert.Contains(t, out, "error") {
		assert.Equal(t, "not_found", out["error"], "should give a json error")
	}
	if assert.Contains(t, out, "reason") {
		assert.Equal(t, "wrong_doctype", out["reason"], "should give a reason")
	}

}

func TestWrongID(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/data/"+TYPE+"/NOTID", nil)
	req.Header.Add("Host", HOST)
	out, res, err := doRequest(req)
	assert.NoError(t, err)
	assert.Equal(t, "404 Not Found", res.Status, "should get a 404")
	if assert.Contains(t, out, "error") {
		assert.Equal(t, "not_found", out["error"], "should give a json error")
	}
	if assert.Contains(t, out, "reason") {
		assert.Equal(t, "missing", out["reason"], "should give a reason")
	}
}

// @TODO uncomment me when we stop falling back to Host = dev
//
// func TestWrongHost(t *testing.T) {
// 	req, _ := http.NewRequest("GET", ts.URL+"/data/"+TYPE+"/"+ID, nil)
// 	req.Header.Add("Host", "NOTHOST")
// 	out, res, err := doRequest(req)
// 	assert.NoError(t, err)
// 	assert.Equal(t, "404 Not Found", res.Status, "should get a 404")
// 	if assert.Contains(t, out, "error") {
// 		assert.Equal(t, "not_found", out["error"], "should give a json error")
// 	}
// 	if assert.Contains(t, out, "reason") {
// 		assert.Equal(t, "wrong_doctype", out["reason"], "should give a reason")
// 	}
// }

func TestSuccessCreate(t *testing.T) {
	var in = jsonReader(&map[string]interface{}{
		"somefield": "avalue",
	})
	req, _ := http.NewRequest("POST", ts.URL+"/data/"+TYPE+"/", in)
	req.Header.Add("Host", HOST)
	out, res, err := doRequest(req)
	assert.NoError(t, err)
	assert.Equal(t, "201 Created", res.Status, "should get a 201")
	assert.Contains(t, out, "ok", "ok at top level (couchdb compatibility)")
	assert.Equal(t, out["ok"], true, "ok is true")
	assert.Contains(t, out, "id", "id at top level (couchdb compatibility)")
	assert.Contains(t, out, "rev", "rev at top level (couchdb compatibility)")
	if assert.Contains(t, out, "data", "document included") {
		data, ismap := out["data"].(map[string]interface{})
		if assert.True(t, ismap, "document is a json object") {
			assert.Contains(t, out["data"], "_id", "document contains _id")
			assert.Contains(t, out["data"], "_rev", "document contains _rev")
			if assert.Contains(t, out["data"], "somefield") {
				assert.Equal(t, data["somefield"], "avalue", "document contains fields")
			}
		}
	}
}

func TestSuccessUpdate(t *testing.T) {

	// Get revision
	get, _ := http.NewRequest("GET", ts.URL+"/data/"+TYPE+"/"+ID, nil)
	doc, res, err := doRequest(get)

	// update it
	var in = jsonReader(&map[string]interface{}{
		"_id":       doc["_id"],
		"_rev":      doc["_rev"],
		"test":      doc["test"],
		"somefield": "anewvalue",
	})
	req, _ := http.NewRequest("PUT", ts.URL+"/data/"+TYPE+"/"+ID, in)
	req.Header.Add("Host", HOST)
	out, res, err := doRequest(req)
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status, "should get a 201")
	assert.Contains(t, out, "ok", "ok at top level (couchdb compatibility)")
	assert.Equal(t, out["ok"], true, "ok is true")
	assert.Contains(t, out, "id", "id at top level (couchdb compatibility)")
	assert.Contains(t, out, "rev", "rev at top level (couchdb compatibility)")
	if assert.Contains(t, out, "data", "document included") {
		data, ismap := out["data"].(map[string]interface{})
		if assert.True(t, ismap, "document is a json object") {
			assert.Contains(t, data, "_id", "document contains _id")
			assert.Contains(t, data, "_rev", "document contains _rev")
			if assert.Contains(t, data, "test") {
				assert.Equal(t, data["test"], "testvalue", "document contains old fields")
			}
			if assert.Contains(t, data, "somefield") {
				assert.Equal(t, data["somefield"], "anewvalue", "document contains new fields")
			}
		}
	}
}

func TestSuccessDelete(t *testing.T) {
	// Get revision
	get, _ := http.NewRequest("GET", ts.URL+"/data/"+TYPE+"/"+ID, nil)
	doc, res, err := doRequest(get)
	rev := doc["_rev"].(string)

	// Do deletion
	req, _ := http.NewRequest("DELETE", ts.URL+"/data/"+TYPE+"/"+ID, nil)
	req.Header.Add("If-Match", rev)
	req.Header.Add("Host", HOST)
	out, res, err := doRequest(req)
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status, "should get a 201")
	assert.Contains(t, out, "ok", "ok at top level (couchdb compatibility)")
	assert.Equal(t, out["ok"], true, "ok is true")
	assert.Contains(t, out, "id", "id at top level (couchdb compatibility)")
	assert.Contains(t, out, "rev", "rev at top level (couchdb compatibility)")
}
