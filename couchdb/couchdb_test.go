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

	// Create the document
	err = CreateDoc(TESTPREFIX, "io.cozy.testobject", doc)
	assert.NoError(t, err)
	assert.NotEmpty(t, doc.Rev(), doc.ID())

	docType, id := doc.DocType(), doc.ID()

	// Fetch it and see if its match
	fetched := &testDoc{}
	err = GetDoc(TESTPREFIX, docType, id, fetched)
	assert.NoError(t, err)
	assert.Equal(t, doc.ID(), fetched.ID())
	assert.Equal(t, doc.Rev(), fetched.Rev())
	assert.Equal(t, "somevalue", fetched.Test)

	revBackup := fetched.Rev()

	// Update it
	updated := fetched
	updated.Test = "changedvalue"
	err = UpdateDoc(TESTPREFIX, updated)
	assert.NoError(t, err)
	assert.NotEqual(t, revBackup, updated.Rev())
	assert.Equal(t, "changedvalue", updated.Test)

	// Refetch it and see if its match
	fetched2 := &testDoc{}
	err = GetDoc(TESTPREFIX, docType, id, fetched2)
	assert.NoError(t, err)
	assert.Equal(t, doc.ID(), fetched2.ID())
	assert.Equal(t, updated.Rev(), fetched2.Rev())
	assert.Equal(t, "changedvalue", fetched2.Test)

	// Delete it
	err = DeleteDoc(TESTPREFIX, updated)
	assert.NoError(t, err)

	fetched3 := &testDoc{}
	err = GetDoc(TESTPREFIX, docType, id, fetched3)
	assert.Error(t, err)
	coucherr, iscoucherr := err.(*Error)
	if assert.True(t, iscoucherr) {
		assert.Equal(t, coucherr.Reason, "deleted")
	}

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
