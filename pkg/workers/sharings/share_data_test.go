package sharings

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"net/url"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/stack"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

var testDocType = "io.cozy.tests"
var testDocID = "aydiayda"

var in *instance.Instance
var domainSharer = "domain.sharer"
var setup *testutils.TestSetup
var ts *httptest.Server

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

// WARNING: this test creates a new testServer. It might conflict with others,
// if others were declared.
func TestDeleteDoc(t *testing.T) {
	randomrev := "randomrev"

	mpr := map[string]func(*echo.Group){
		"/sharings": func(router *echo.Group) {
			router.Any("/doc/:doctype/:docid", func(c echo.Context) error {
				assert.Equal(t, randomrev, c.QueryParam("rev"))
				assert.Equal(t, testDocID, c.Param("docid"))
				assert.Equal(t, testDocType, c.Param("doctype"))
				return c.JSON(http.StatusOK, nil)
			})
		},
		"/data": func(router *echo.Group) {
			router.Any("/:doctype/:docid", func(c echo.Context) error {
				assert.Equal(t, testDocID, c.Param("docid"))
				assert.Equal(t, testDocType, c.Param("doctype"))
				doc := &couchdb.JSONDoc{
					Type: testDocType,
					M: map[string]interface{}{
						"_rev": randomrev,
					},
				}
				return c.JSON(http.StatusOK, doc.ToMapWithType())
			})
		},
	}
	ts = setup.GetTestServerMultipleRoutes(mpr)

	tsURL, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	domain := tsURL.Host
	opts := &SendOptions{
		DocID:   testDocID,
		DocType: testDocType,
		Method:  http.MethodDelete,
		Recipients: []*RecipientInfo{
			&RecipientInfo{
				URL:   domain,
				Token: "whoneedsone?",
			},
		},
	}

	err = DeleteDoc(domain, opts)
	assert.NoError(t, err)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()

	setup = testutils.NewSetup(m, "share_data_test")

	err := stack.Start()
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
