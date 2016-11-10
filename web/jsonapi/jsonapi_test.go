package jsonapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

var ts *httptest.Server

type Foo struct {
	FID  string `json:"-"`
	FRev string `json:"-"`
	Bar  string `json:"bar"`
}

func (f *Foo) ID() string {
	return f.FID
}

func (f *Foo) Rev() string {
	return f.FRev
}

func (f *Foo) DocType() string {
	return "io.cozy.foos"
}

func (f *Foo) SetID(id string) {
	f.FID = id
}

func (f *Foo) SetRev(rev string) {
	f.FRev = rev
}

func (f *Foo) SelfLink() string {
	return "/foos/" + f.FID
}

func (f *Foo) Relationships() RelationshipMap {
	qux := map[string]string{
		"type": "io.cozy.foos",
		"id":   "qux",
	}
	single := Relationship{
		Links: &LinksList{
			Related: "/foos/" + f.FID + "/single",
		},
		Data: qux,
	}
	multiple := Relationship{
		Links: &LinksList{
			Related: "/foos/" + f.FID + "/multiple",
		},
		Data: []map[string]string{qux},
	}
	return RelationshipMap{
		"single":   single,
		"multiple": multiple,
	}
}

func (f *Foo) Included() []Object {
	qux := &Foo{FID: "qux", FRev: "42-xyz", Bar: "quux"}
	return []Object{qux}
}

func TestObjectMarshalling(t *testing.T) {
	foo := &Foo{FID: "courge", FRev: "1-abc", Bar: "baz"}
	raw, err := MarshalObject(foo)
	assert.NoError(t, err)
	var data map[string]interface{}
	err = json.Unmarshal(raw, &data)

	assert.NoError(t, err)
	assert.Equal(t, data["type"], "io.cozy.foos")
	assert.Equal(t, data["id"], "courge")
	assert.Contains(t, data, "meta")
	meta, _ := data["meta"].(map[string]interface{})
	assert.Equal(t, meta["rev"], "1-abc")
	assert.Contains(t, data, "attributes")
	attrs, _ := data["attributes"].(map[string]interface{})
	assert.Equal(t, attrs["bar"], "baz")
	assert.Contains(t, data, "links")
	links, _ := data["links"].(map[string]interface{})
	assert.Equal(t, links["self"], "/foos/courge")

	assert.Contains(t, data, "relationships")
	rels, _ := data["relationships"].(map[string]interface{})
	assert.Contains(t, rels, "single")
	single, _ := rels["single"].(map[string]interface{})
	assert.Contains(t, single, "links")
	links1, _ := single["links"].(map[string]interface{})
	assert.Equal(t, links1["related"], "/foos/courge/single")
	assert.Contains(t, single, "data")
	data1, _ := single["data"].(map[string]interface{})
	assert.Equal(t, data1["type"], "io.cozy.foos")
	assert.Equal(t, data1["id"], "qux")

	assert.Contains(t, rels, "multiple")
	multiple, _ := rels["multiple"].(map[string]interface{})
	assert.Contains(t, multiple, "links")
	links2, _ := multiple["links"].(map[string]interface{})
	assert.Equal(t, links2["related"], "/foos/courge/multiple")
	assert.Contains(t, multiple, "data")
	data2, _ := multiple["data"].([]interface{})
	assert.Len(t, data2, 1)
	qux, _ := data2[0].(map[string]interface{})
	assert.Equal(t, qux["type"], "io.cozy.foos")
	assert.Equal(t, qux["id"], "qux")
}

func TestData(t *testing.T) {
	res, err := http.Get(ts.URL + "/foos/courge")
	assert.NoError(t, err)
	assert.Equal(t, "200 OK", res.Status, "should get a 200")
	assert.Equal(t, "application/vnd.api+json", res.Header.Get("Content-Type"))
	defer res.Body.Close()
	var body map[string]interface{}
	json.NewDecoder(res.Body).Decode(&body)

	assert.Contains(t, body, "data")
	data := body["data"].(map[string]interface{})
	assert.Equal(t, data["type"], "io.cozy.foos")
	assert.Equal(t, data["id"], "courge")
	assert.Contains(t, data, "attributes")
	assert.Contains(t, data, "relationships")
	assert.Contains(t, data, "links")

	assert.Contains(t, body, "included")
	included := body["included"].([]interface{})
	assert.Len(t, included, 1)
	qux, _ := included[0].(map[string]interface{})
	assert.Equal(t, qux["type"], "io.cozy.foos")
	assert.Equal(t, qux["id"], "qux")
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/foos/courge", func(c *gin.Context) {
		courge := &Foo{FID: "courge", FRev: "1-abc", Bar: "baz"}
		Data(c, 200, courge, nil)
	})
	ts = httptest.NewServer(router)
	defer ts.Close()
	os.Exit(m.Run())
}
