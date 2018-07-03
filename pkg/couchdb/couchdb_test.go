package couchdb

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"
	"unsafe"

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

var TestPrefix = newDatabase("couchdb-tests")
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

	olddocs := make([]interface{}, len(results))
	docs := make([]interface{}, len(results))
	for i, doc := range results {
		docs[i] = doc
	}
	err = BulkUpdateDocs(TestPrefix, results[0].DocType(), docs, olddocs)
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

func TestEnsureDBExist(t *testing.T) {
	defer DeleteDB(TestPrefix, "io.cozy.tests.db1")
	_, err := DBStatus(TestPrefix, "io.cozy.tests.db1")
	assert.True(t, IsNoDatabaseError(err))
	assert.NoError(t, EnsureDBExist(TestPrefix, "io.cozy.tests.db1"))
	_, err = DBStatus(TestPrefix, "io.cozy.tests.db1")
	assert.NoError(t, err)
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
	eventChan := realtime.GetHub().Subscriber(TestPrefix)
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

func TestJSONDocClone(t *testing.T) {
	var m map[string]interface{}
	data := []byte(`{
	"foo1": "bar",
	"foo2": [0,1,2,3],
	"foo3": ["abc", 1, 1.1],
	"foo4": {
		"bar1":"bar",
		"bar2": [0,1,2,3],
		"bar3": ["abc", 1, 1.1, { "key": "value", "key2": [{}, 1, 2, 3] }],
		"bar4": {}
	},
	"foo5": 1,
	"foo6": 0.001,
	"foo7": "toto"
}`)

	err := json.Unmarshal(data, &m)
	assert.NoError(t, err)
	j1 := JSONDoc{
		Type: "toto",
		M:    m,
	}
	j2 := j1.Clone().(JSONDoc)

	assert.Equal(t, j1.Type, j2.Type)
	assert.True(t, reflect.DeepEqual(j1.M, j2.M))

	assert.False(t, reflect.ValueOf(j1.M["foo2"]).Pointer() == reflect.ValueOf(j2.M["foo2"]).Pointer())
	assert.False(t, reflect.ValueOf(j1.M["foo3"]).Pointer() == reflect.ValueOf(j2.M["foo3"]).Pointer())
	assert.False(t, reflect.ValueOf(j1.M["foo4"]).Pointer() == reflect.ValueOf(j2.M["foo4"]).Pointer())

	s1 := j1.M["foo1"].(string)
	s2 := j2.M["foo1"].(string)
	s3 := j1.M["foo7"].(string)
	s4 := j2.M["foo7"].(string)

	hdr1 := (*reflect.StringHeader)(unsafe.Pointer(&s1))
	hdr2 := (*reflect.StringHeader)(unsafe.Pointer(&s2))
	hdr3 := (*reflect.StringHeader)(unsafe.Pointer(&s3))
	hdr4 := (*reflect.StringHeader)(unsafe.Pointer(&s4))

	assert.Equal(t, hdr1.Data, hdr2.Data)
	assert.Equal(t, hdr1.Len, hdr2.Len)

	assert.Equal(t, hdr3.Data, hdr4.Data)
	assert.Equal(t, hdr3.Len, hdr4.Len)

	assert.NotEqual(t, hdr1.Data, hdr4.Data)
	assert.NotEqual(t, hdr1.Len, hdr4.Len)
}

func TestLocalDocuments(t *testing.T) {
	id := "foo"
	_, err := GetLocal(TestPrefix, TestDoctype, id)
	assert.True(t, IsNotFoundError(err))

	doc := map[string]interface{}{"bar": "baz"}
	err = PutLocal(TestPrefix, TestDoctype, id, doc)
	assert.NoError(t, err)
	assert.NotEmpty(t, doc["_rev"])

	out, err := GetLocal(TestPrefix, TestDoctype, id)
	assert.NoError(t, err)
	assert.Equal(t, "baz", out["bar"])

	err = DeleteLocal(TestPrefix, TestDoctype, id)
	assert.NoError(t, err)

	_, err = GetLocal(TestPrefix, TestDoctype, id)
	assert.True(t, IsNotFoundError(err))
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
