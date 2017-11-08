package couchdb

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cozy/checkup"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/stretchr/testify/assert"
)

func TestErrors(t *testing.T) {
	err := Error{StatusCode: 404, Name: "not_found", Reason: "missing"}
	assert.Contains(t, err.Error(), "not_found")
	assert.Contains(t, err.Error(), "missing")
}

const TestDoctype = "io.cozy.testobject"

var TestPrefix = SimpleDatabasePrefix("couchdb-tests")
var receivedEventsMutex sync.Mutex
var receivedEvents map[string]struct{}

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

func (t *testDoc) Clone() Doc {
	cloned := *t
	return &cloned
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
	assertGotEvent(t, realtime.EventCreate, doc.ID())

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
	assertGotEvent(t, realtime.EventUpdate, doc.ID())

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
	assertGotEvent(t, realtime.EventDelete, doc.ID())

	fetched3 := &testDoc{}
	err = GetDoc(TestPrefix, docType, id, fetched3)
	assert.Error(t, err)
	coucherr, iscoucherr := err.(*Error)
	if assert.True(t, iscoucherr) {
		assert.Equal(t, coucherr.Reason, "deleted")
	}
}

func TestGetAllDocs(t *testing.T) {
	doc1 := &testDoc{Test: "all_1"}
	doc2 := &testDoc{Test: "all_2"}
	CreateDoc(TestPrefix, doc1)
	CreateDoc(TestPrefix, doc2)

	var results []*testDoc
	err := GetAllDocs(TestPrefix, TestDoctype, &AllDocsRequest{Limit: 2}, &results)
	if assert.NoError(t, err) {
		assert.Len(t, results, 2)
		assert.Equal(t, results[0].Test, "all_1")
		assert.Equal(t, results[1].Test, "all_2")
	}
}

func TestBulkUpdateDocs(t *testing.T) {
	doc1 := &testDoc{Test: "before_1"}
	doc2 := &testDoc{Test: "before_2"}
	CreateDoc(TestPrefix, doc1)
	CreateDoc(TestPrefix, doc2)

	var results []*testDoc
	err := GetAllDocs(TestPrefix, TestDoctype, &AllDocsRequest{Limit: 2}, &results)
	assert.NoError(t, err)
	results[0].Test = "after_1"
	results[1].Test = "after_2"

	docs := make([]interface{}, len(results))
	for i, doc := range results {
		docs[i] = doc
	}
	err = BulkUpdateDocs(TestPrefix, results[0].DocType(), docs)
	assert.NoError(t, err)

	err = GetAllDocs(TestPrefix, TestDoctype, &AllDocsRequest{Limit: 2}, &results)
	if assert.NoError(t, err) {
		assert.Len(t, results, 2)
		assert.Equal(t, results[0].Test, "after_1")
		assert.Equal(t, results[1].Test, "after_2")
	}
}

func TestDefineIndex(t *testing.T) {
	err := DefineIndex(TestPrefix, mango.IndexOnFields(TestDoctype, "my-index", []string{"fieldA", "fieldB"}))
	assert.NoError(t, err)

	// if I try to define the same index several time
	err2 := DefineIndex(TestPrefix, mango.IndexOnFields(TestDoctype, "my-index", []string{"fieldA", "fieldB"}))
	assert.NoError(t, err2)
}

func TestQuery(t *testing.T) {

	// create a few docs for testing
	doc1 := testDoc{FieldA: "value1", FieldB: 100}
	doc2 := testDoc{FieldA: "value2", FieldB: 1000}
	doc3 := testDoc{FieldA: "value2", FieldB: 300}
	doc4 := testDoc{FieldA: "value1", FieldB: 1500}
	doc5 := testDoc{FieldA: "value1", FieldB: 150}
	docs := []*testDoc{&doc1, &doc2, &doc3, &doc4, &doc5}
	for _, doc := range docs {
		err := CreateDoc(TestPrefix, doc)
		if !assert.NoError(t, err) || doc.ID() == "" {
			t.FailNow()
			return
		}
	}

	err := DefineIndex(TestPrefix, mango.IndexOnFields(TestDoctype, "my-index", []string{"fieldA", "fieldB"}))
	if !assert.NoError(t, err) {
		t.FailNow()
		return
	}
	var out []testDoc
	req := &FindRequest{
		UseIndex: "my-index",
		Selector: mango.And(
			mango.Equal("fieldA", "value2"),
			mango.Exists("fieldB"),
		),
	}
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
	req2 := &FindRequest{
		UseIndex: "my-index",
		Selector: mango.And(
			mango.Equal("fieldA", "value1"),
			mango.Between("fieldB", 10, 1000),
		),
	}
	err = FindDocs(TestPrefix, TestDoctype, req2, &out2)
	if assert.NoError(t, err) {
		assert.Len(t, out, 2, "should get 2 results")
		assert.Equal(t, doc1.ID(), out2[0].ID())
		assert.Equal(t, doc5.ID(), out2[1].ID())
	}

}

func TestChangesSuccess(t *testing.T) {
	err := ResetDB(TestPrefix, TestDoctype)
	assert.NoError(t, err)

	var request = &ChangesRequest{
		DocType: TestDoctype,
	}
	response, err := GetChanges(TestPrefix, request)
	var seqnoAfterCreates = response.LastSeq
	assert.NoError(t, err)
	assert.Len(t, response.Results, 0)

	doc1 := makeTestDoc()
	doc2 := makeTestDoc()
	doc3 := makeTestDoc()
	CreateDoc(TestPrefix, doc1)
	CreateDoc(TestPrefix, doc2)
	CreateDoc(TestPrefix, doc3)

	request = &ChangesRequest{
		DocType: TestDoctype,
		Since:   seqnoAfterCreates,
	}

	response, err = GetChanges(TestPrefix, request)
	assert.NoError(t, err)
	assert.Len(t, response.Results, 3)

	request = &ChangesRequest{
		DocType: TestDoctype,
		Since:   seqnoAfterCreates,
		Limit:   2,
	}

	response, err = GetChanges(TestPrefix, request)
	assert.NoError(t, err)
	assert.Len(t, response.Results, 2)

	seqnoAfterCreates = response.LastSeq

	doc4 := makeTestDoc()
	CreateDoc(TestPrefix, doc4)

	request = &ChangesRequest{
		DocType: TestDoctype,
		Since:   seqnoAfterCreates,
	}
	response, err = GetChanges(TestPrefix, request)
	assert.NoError(t, err)
	assert.Len(t, response.Results, 2)
}

func TestMain(m *testing.M) {
	config.UseTestFile()

	// First we make sure couchdb is started
	db, err := checkup.HTTPChecker{URL: config.CouchURL().String()}.Check()
	if err != nil || db.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}

	err = ResetDB(TestPrefix, TestDoctype)
	if err != nil {
		fmt.Printf("Cant reset db (%s, %s) %s\n", TestPrefix, TestDoctype, err.Error())
		os.Exit(1)
	}

	receivedEvents = make(map[string]struct{})
	eventChan := realtime.GetHub().Subscriber(TestPrefix.Prefix())
	eventChan.Subscribe(TestDoctype)
	go func() {
		for ev := range eventChan.Channel {
			receivedEventsMutex.Lock()
			receivedEvents[ev.Verb+ev.Doc.ID()] = struct{}{}
			receivedEventsMutex.Unlock()
		}
	}()

	res := m.Run()

	eventChan.Close()

	DeleteDB(TestPrefix, TestDoctype)

	os.Exit(res)
}

func assertGotEvent(t *testing.T, eventType, id string) bool {
	receivedEventsMutex.Lock()
	_, ok := receivedEvents[eventType+id]
	if !ok {
		receivedEventsMutex.Unlock()
		time.Sleep(time.Millisecond)
		receivedEventsMutex.Lock()
		_, ok = receivedEvents[eventType+id]
	}

	if ok {
		delete(receivedEvents, eventType+id)
	}
	receivedEventsMutex.Unlock()

	return assert.True(t, ok, "Expected event %s:%s", eventType, id)
}
