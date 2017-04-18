package sharings

import (
	"fmt"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/stretchr/testify/assert"
)

var testDocType = "io.cozy.tests"
var testDocID = "aydiayda"

var in *instance.Instance
var domainSharer = "domain.sharer"

func createInstance(domain, publicName string) (*instance.Instance, error) {
	var settings couchdb.JSONDoc
	settings.M = make(map[string]interface{})
	settings.M["public_name"] = publicName
	opts := &instance.Options{
		Domain:   domain,
		Settings: settings,
	}
	return instance.Create(opts)
}

func TestSendDataMissingDocType(t *testing.T) {
	docType := "fakedoctype"
	err := couchdb.ResetDB(in, docType)
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

	err = SendData(jobs.NewWorkerContext(domainSharer, "123"), msg)
	assert.Error(t, err)
	assert.Equal(t, "CouchDB(not_found): missing", err.Error())
}

func TestSendDataBadID(t *testing.T) {

	doc := &couchdb.JSONDoc{
		Type: testDocType,
		M:    map[string]interface{}{"test": "tests"},
	}
	doc.SetID(testDocID)
	err := couchdb.CreateNamedDocWithDB(in, doc)
	assert.NoError(t, err)
	defer func() {
		couchdb.DeleteDoc(in, doc)
	}()

	msg, err := jobs.NewMessage(jobs.JSONEncoding, SendOptions{
		DocID:      "fakeid",
		DocType:    testDocType,
		Recipients: []*RecipientInfo{},
	})
	assert.NoError(t, err)

	err = SendData(jobs.NewWorkerContext(domainSharer, "123"), msg)
	assert.Error(t, err)
	assert.Equal(t, "CouchDB(not_found): missing", err.Error())
}

func TestSendDataBadRecipient(t *testing.T) {

	doc := &couchdb.JSONDoc{
		Type: testDocType,
		M:    map[string]interface{}{"test": "tests"},
	}
	doc.SetID(testDocID)
	err := couchdb.CreateNamedDocWithDB(in, doc)
	assert.NoError(t, err)
	defer func() {
		couchdb.DeleteDoc(in, doc)
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

	err = SendData(jobs.NewWorkerContext(domainSharer, "123"), msg)
	assert.NoError(t, err)
}

func TestMain(m *testing.M) {
	config.UseTestFile()

	err := jobs.StartSystem()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	_, _ = instance.Destroy(domainSharer)
	in, err = createInstance(domainSharer, "Alice")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = couchdb.ResetDB(in, testDocType)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = couchdb.ResetDB(in, consts.Sharings)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	err = couchdb.DefineIndex(in, mango.IndexOnFields(consts.Sharings, "by-sharing-id", []string{"sharing_id"}))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}
