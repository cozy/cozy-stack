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
	TestID  string `json:"_id"`
	TestRev string `json:"_rev,omitempty"`
	Test    string `json:"test"`
}

func (t *testDoc) ID() string {
	return t.TestID
}

func (t *testDoc) Rev() string {
	return t.TestRev
}

func (t *testDoc) DocType() string {
	return "io.cozy.testobject"
}

func (t *testDoc) SetID(id string) {
	t.TestID = id
}

func (t *testDoc) SetRev(rev string) {
	t.TestRev = rev
}

func makeTestDoc() Doc {
	return &testDoc{
		Test: "somevalue",
	}
}

func TestCreateDoc(t *testing.T) {
	var err error

	var TESTPREFIX = "dev/"
	var doc = makeTestDoc()
	assert.Empty(t, doc.Rev(), doc.ID())
	err = CreateDoc(TESTPREFIX, "io.cozy.testobject", doc)
	assert.NoError(t, err)
	assert.NotEmpty(t, doc.Rev(), doc.ID())

	docType, id := doc.DocType(), doc.ID()

	out := &testDoc{}
	err = GetDoc(TESTPREFIX, docType, id, out)
	assert.NoError(t, err)
	assert.Equal(t, out.ID(), doc.ID())
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
