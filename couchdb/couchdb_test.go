package couchdb

import (
	"testing"

	"github.com/sourcegraph/checkup"
	"github.com/stretchr/testify/assert"
)

var CouchDBURL = "http://localhost:5984/"

func TestErrors(t *testing.T) {
	body := []byte("{\"reason\": missing}")
	err := Error{404, body}
	assert.Contains(t, err.Error(), "404")
	assert.Contains(t, err.Error(), "missing")
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
	var TESTDOC = map[string]string{
		"test": "somevalue",
	}

	// CreateDoc()

}
