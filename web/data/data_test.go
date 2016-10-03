package data

import (
	"bytes"
	"io"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

var client = &http.Client{}
var DOCUMENT = map[string]string{
	"_id": TYPE + "/" + ID,
	"test": "testvalue",
}
const HOST = "example.com"
const TYPE = "io.cozy.events"
const ID = "4521C325F6478E45"
const EXPECTEDDBNAME = "example-com%2Fio-cozy-events"

// @TODO this should be moved to our couchdb package or to
// some test helpers files.

func couchReq(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, "http://localhost:5984/" + path, body)
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
			*http.Response, []byte, error){

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

// prepareCouch destroy and re-create the database
func prepareCouchdb(t *testing.T){
	_, err := couchReq("DELETE", EXPECTEDDBNAME, nil)
	assert.NoError(t, err)

	_, err = couchReq("PUT", EXPECTEDDBNAME, nil)
	assert.NoError(t, err)

	jsonbytes, _ := json.Marshal(DOCUMENT)
	reqbody := bytes.NewReader(jsonbytes)
	_, err = couchReq("PUT", EXPECTEDDBNAME + "/" + TYPE + "%2F" + ID, reqbody)
	assert.NoError(t, err)
}


func injectInstance(instance *middlewares.Instance) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("instance", instance)
	}
}

func TestRoutes(t *testing.T) {
	router := gin.New()
	instance := &middlewares.Instance{
		Domain:     HOST,
		StorageURL: "mem://test",
	}
	router.Use(gin.ErrorLogger())
	router.Use(injectInstance(instance))
	Routes(router.Group("/data"))
	ts := httptest.NewServer(router)
	defer ts.Close()

	prepareCouchdb(t)

	var out interface{}
	res, _, err := testRoute(t, ts.URL + "/data/" + TYPE + "/" + ID, HOST, &out)
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status, "should get a 200")
	jsonmap, ok := out.(map[string]interface{})
	if assert.NotNil(t, ok, "should contains json") {
		assert.Equal(t, jsonmap["test"], "testvalue", "should give the same doc")
	}

	var out2 interface{}
	res, _, err = testRoute(t, ts.URL + "/data/nottype/" + ID, HOST, &out2)
	assert.NoError(t, err)
	assert.Equal(t, "404 Not Found", res.Status, "should get a 404")

	var out3 interface{}
	res, _, err = testRoute(t, ts.URL + "/data/" + TYPE + "/NOTID", HOST, &out3)
	assert.NoError(t, err)
	assert.Equal(t, "404 Not Found", res.Status, "should get a 404")
}
