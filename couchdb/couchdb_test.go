package couchdb

import (
	"fmt"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/couchdb/mango"
	"github.com/sourcegraph/checkup"
	"github.com/stretchr/testify/assert"
)

func TestErrors(t *testing.T) {
	err := Error{StatusCode: 404, Name: "not_found", Reason: "missing"}
	assert.Contains(t, err.Error(), "404")
	assert.Contains(t, err.Error(), "missing")
}

const TestDoctype = "io.cozy.testobject"
const TestPrefix = "dev/"

type testDoc struct {
	TestID  string `json:"_id,omitempty"`
	TestRev string `json:"_rev,omitempty"`
	Test    string `json:"test"`
	FieldA  string `json:"fieldA,omitempty"`
	FieldB  int    `json:"fieldB,omitempty"`
}

func (t *testDoc) ID() string {
	return t.TestID
}

func (t *testDoc) Rev() string {
	return t.TestRev
}

func (t *testDoc) DocType() string {
	return TestDoctype
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

	var doc = makeTestDoc()
	assert.Empty(t, doc.Rev(), doc.ID())

	// Create the document
	err = CreateDoc(TestPrefix, doc)
	assert.NoError(t, err)
	assert.NotEmpty(t, doc.Rev(), doc.ID())

	docType, id := doc.DocType(), doc.ID()

	// Fetch it and see if its match
	fetched := &testDoc{}
	err = GetDoc(TestPrefix, docType, id, fetched)
	assert.NoError(t, err)
	assert.Equal(t, doc.ID(), fetched.ID())
	assert.Equal(t, doc.Rev(), fetched.Rev())
	assert.Equal(t, "somevalue", fetched.Test)

	revBackup := fetched.Rev()

	// Update it
	updated := fetched
	updated.Test = "changedvalue"
	err = UpdateDoc(TestPrefix, updated)
	assert.NoError(t, err)
	assert.NotEqual(t, revBackup, updated.Rev())
	assert.Equal(t, "changedvalue", updated.Test)

	// Refetch it and see if its match
	fetched2 := &testDoc{}
	err = GetDoc(TestPrefix, docType, id, fetched2)
	assert.NoError(t, err)
	assert.Equal(t, doc.ID(), fetched2.ID())
	assert.Equal(t, updated.Rev(), fetched2.Rev())
	assert.Equal(t, "changedvalue", fetched2.Test)

	// Delete it
	err = DeleteDoc(TestPrefix, updated)
	assert.NoError(t, err)

	fetched3 := &testDoc{}
	err = GetDoc(TestPrefix, docType, id, fetched3)
	assert.Error(t, err)
	coucherr, iscoucherr := err.(*Error)
	if assert.True(t, iscoucherr) {
		assert.Equal(t, coucherr.Reason, "deleted")
	}

}

func TestDefineIndex(t *testing.T) {
	err := DefineIndex(TestPrefix, TestDoctype, mango.IndexOnFields("fieldA", "fieldB"))
	assert.NoError(t, err)

	// if I try to define the same index several time
	err2 := DefineIndex(TestPrefix, TestDoctype, mango.IndexOnFields("fieldA", "fieldB"))
	assert.NoError(t, err2)
}

func TestQuery(t *testing.T) {

	// create a few docs for testing
	doc1 := testDoc{FieldA: "value1", FieldB: 100}
	doc2 := testDoc{FieldA: "value2", FieldB: 1000}
	doc3 := testDoc{FieldA: "value2", FieldB: 300}
	doc4 := testDoc{FieldA: "value13", FieldB: 1500}
	docs := []*testDoc{&doc1, &doc2, &doc3, &doc4}
	for _, doc := range docs {
		err := CreateDoc(TestPrefix, doc)
		if !assert.NoError(t, err) || doc.ID() == "" {
			t.FailNow()
			return
		}
	}

	err := DefineIndex(TestPrefix, TestDoctype, mango.IndexOnFields("fieldA", "fieldB"))
	if !assert.NoError(t, err) {
		t.FailNow()
		return
	}
	var out []testDoc
	req := &FindRequest{Selector: mango.Equal("fieldA", "value2")}
	err = FindDocs(TestPrefix, TestDoctype, req, &out)
	if assert.NoError(t, err) {
		assert.Len(t, out, 2, "should get 2 results")
		// if fieldA are equaly, docs will be ordered by fieldB
		assert.Equal(t, doc3.ID(), out[0].ID())
		assert.Equal(t, "value2", out[0].FieldA)
		assert.Equal(t, doc2.ID(), out[1].ID())
		assert.Equal(t, "value2", out[1].FieldA)
	}

	var out2 []testDoc
	req2 := &FindRequest{Selector: mango.StartWith("fieldA", "value1")}
	err = FindDocs(TestPrefix, TestDoctype, req2, &out2)
	if assert.NoError(t, err) {
		assert.Len(t, out, 2, "should get 2 results")
		// if we do as startWith, docs will be ordered by the rest of fieldA
		assert.Equal(t, doc1.ID(), out2[0].ID())
		assert.Equal(t, doc4.ID(), out2[1].ID())
	}

}

func TestMain(m *testing.M) {
	config.UseTestFile()

	// First we make sure couchdb is started
	db, err := checkup.HTTPChecker{URL: config.CouchURL()}.Check()
	if err != nil || db.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}

	err = ResetDB(TestPrefix, TestDoctype)
	if err != nil {
		fmt.Printf("Cant reset db (%s, %s) %s\n", TestPrefix, TestDoctype, err.Error())
		os.Exit(1)
	}

	os.Exit(m.Run())
}
