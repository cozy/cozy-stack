package couchdb

import (
	"testing"

	"github.com/sourcegraph/checkup"
	"github.com/stretchr/testify/assert"
)

var CouchDBURL = "http://localhost:5984/"

func TestErrors(t *testing.T) {
	err := Error{StatusCode: 404, Name: "not_found", Reason: "missing"}
	assert.Contains(t, err.Error(), "404")
	assert.Contains(t, err.Error(), "missing")
}

func makeTestDoc() Doc {
	return map[string]interface{}{
		"test": "somevalue",
	}
}

func TestMain(t *testing.T) {

	TestErrors(t)

	// First we make sure couchdb is started
	couchdb, err := checkup.HTTPChecker{URL: CouchDBURL}.Check()
	if err != nil || couchdb.Status() != checkup.Healthy {
		t.Fatal("This test need couchdb to run.")
	}

	var TESTPREFIX = "dev/"
	var TESTTYPE = "io.cozy.testobject"
	var doc = makeTestDoc()
	err = CreateDoc(TESTPREFIX, TESTTYPE, doc)
	assert.NoError(t, err)
	assert.NotEmpty(t, doc["_id"])
	docType, id := doc.GetDoctypeAndID()
	var out Doc
	err = GetDoc(TESTPREFIX, docType, id, &out)
	assert.NoError(t, err)
	assert.Equal(t, out["_id"], doc["_id"])
	assert.Equal(t, out["test"], "somevalue")

}
