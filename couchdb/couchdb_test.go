package couchdb

import (
	"fmt"
	"os"
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

type testDoc struct {
	ID   string `json:"_id"`
	Test string `json:"test"`
}

func (t *testDoc) GetID() string {
	return t.ID
}

func (t *testDoc) SetID(id string) {
	t.ID = id
}

func makeTestDoc() Doc {
	return &testDoc{
		Test: "somevalue",
	}
}

func TestCreateDoc(t *testing.T) {
	var TESTPREFIX = "dev/"
	var TESTTYPE = "io.cozy.testobject"
	var doc = makeTestDoc()
	assert.Empty(t, doc.GetID())
	rev, err := CreateDoc(TESTPREFIX, TESTTYPE, doc)
	assert.NoError(t, err)
	assert.NotEmpty(t, rev, doc.GetID())

	docType, id := GetDoctypeAndID(doc)

	out := &testDoc{}
	err = GetDoc(TESTPREFIX, docType, id, out)
	assert.NoError(t, err)
	assert.Equal(t, out.GetID(), doc.GetID())
	assert.Equal(t, out.Test, "somevalue")
}

func TestMain(m *testing.M) {
	// First we make sure couchdb is started
	couchdb, err := checkup.HTTPChecker{URL: CouchDBURL}.Check()
	if err != nil || couchdb.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}

	os.Exit(m.Run())
}
