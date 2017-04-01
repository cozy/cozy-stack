package workers

import (
	"fmt"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/stretchr/testify/assert"
)

var testDocType = "io.cozy.tests"
var testDocID = "aydiayda"

func TestSendDataMissingDocType(t *testing.T) {
	domain := "baddatabase.triggers"
	db := couchdb.SimpleDatabasePrefix(domain)
	docType := "fakedoctype"
	err := couchdb.ResetDB(db, docType)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	msg, err := jobs.NewMessage(jobs.JSONEncoding, SendOptions{
		DocID:      "fakeid",
		DocType:    docType,
		Recipients: []*RecipientInfo{},
	})
	assert.NoError(t, err)

	err = SendData(jobs.NewWorkerContext(domain), msg)
	assert.Error(t, err)
	assert.Equal(t, "CouchDB(not_found): missing", err.Error())
}

func TestSendDataBadID(t *testing.T) {
	domain := "badid.triggers"
	db := couchdb.SimpleDatabasePrefix(domain)

	doc := &couchdb.JSONDoc{
		Type: testDocType,
		M:    map[string]interface{}{"test": "tests"},
	}
	doc.SetID(testDocID)
	err := couchdb.CreateNamedDocWithDB(db, doc)
	assert.NoError(t, err)
	defer func() {
		couchdb.DeleteDoc(db, doc)
	}()

	msg, err := jobs.NewMessage(jobs.JSONEncoding, SendOptions{
		DocID:      "fakeid",
		DocType:    testDocType,
		Recipients: []*RecipientInfo{},
	})
	assert.NoError(t, err)

	err = SendData(jobs.NewWorkerContext(domain), msg)
	assert.Error(t, err)
	assert.Equal(t, "CouchDB(not_found): missing", err.Error())
}

func TestSendDataBadRecipient(t *testing.T) {
	domain := "badrecipient.triggers"
	db := couchdb.SimpleDatabasePrefix(domain)

	doc := &couchdb.JSONDoc{
		Type: testDocType,
		M:    map[string]interface{}{"test": "tests"},
	}
	doc.SetID(testDocID)
	err := couchdb.CreateNamedDocWithDB(db, doc)
	assert.NoError(t, err)
	defer func() {
		couchdb.DeleteDoc(db, doc)
	}()

	rec := &RecipientInfo{
		URL:   "nowhere",
		Token: "inthesky",
	}

	msg, err := jobs.NewMessage(jobs.JSONEncoding, SendOptions{
		DocID:      testDocID,
		DocType:    testDocType,
		Recipients: []*RecipientInfo{rec},
	})
	assert.NoError(t, err)

	err = SendData(jobs.NewWorkerContext(domain), msg)
	assert.NoError(t, err)
}
