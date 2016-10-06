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

func testRoute(t *testing.T, url string, host string, jsonout interface{}) (
	*http.Response, []byte, error) {

	fmt.Println("test req", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Add("Host", host)

	res, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()
	body, ioerr := ioutil.ReadAll(res.Body)
	if ioerr != nil {
		return res, nil, nil
	}
	return res, body, json.Unmarshal(body, jsonout)

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
	var out map[string]interface{}
	res, _, err := testRoute(t, ts.URL+"/data/"+TYPE+"/"+ID, HOST, &out)
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status, "should get a 200")
	if assert.Contains(t, out, "test") {
		assert.Equal(t, out["test"], "testvalue", "should give the same doc")
	}
}

func TestWrongDoctype(t *testing.T) {
	var out map[string]interface{}
	res, _, err := testRoute(t, ts.URL+"/data/nottype/"+ID, HOST, &out)
	assert.NoError(t, err)
	fmt.Println("RESULT=", out)
	assert.Equal(t, "404 Not Found", res.Status, "should get a 404")
	if assert.Contains(t, out, "error") {
		assert.Equal(t, "not_found", out["error"], "should give a json error")
	}
	if assert.Contains(t, out, "reason") {
		assert.Equal(t, "wrong_doctype", out["reason"], "should give a reason")
	}

}

func TestWrongID(t *testing.T) {
	var out map[string]interface{}
	res, _, err := testRoute(t, ts.URL+"/data/"+TYPE+"/NOTID", HOST, &out)
	assert.NoError(t, err)
	assert.Equal(t, "404 Not Found", res.Status, "should get a 404")
	if assert.Contains(t, out, "error") {
		assert.Equal(t, "not_found", out["error"], "should give a json error")
	}
	if assert.Contains(t, out, "reason") {
		assert.Equal(t, "missing", out["reason"], "should give a reason")
	}
}
