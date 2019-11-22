package contacts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
)

var ts *httptest.Server
var testInstance *instance.Instance
var token string

func assertMyself(t *testing.T, res *http.Response) {
	assert.Equal(t, 200, res.StatusCode)
	var result map[string]interface{}
	err := json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)

	data, _ := result["data"].(map[string]interface{})
	assert.NotEmpty(t, data["id"])
	assert.Equal(t, consts.Contacts, data["type"])
	meta, _ := data["meta"].(map[string]interface{})
	assert.NotEmpty(t, meta["rev"])

	attrs, _ := data["attributes"].(map[string]interface{})
	assert.Equal(t, "Alice", attrs["fullname"])
	emails, _ := attrs["email"].([]interface{})
	if assert.Len(t, emails, 1) {
		email, _ := emails[0].(map[string]interface{})
		assert.Equal(t, "alice@example.com", email["address"])
		assert.Equal(t, true, email["primary"])
	}
}

func TestMyself(t *testing.T) {
	req, _ := http.NewRequest("POST", ts.URL+"/contacts/myself", nil)
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assertMyself(t, res)

	myself, err := contact.GetMyself(testInstance)
	assert.NoError(t, err)
	err = couchdb.DeleteDoc(testInstance, myself)
	assert.NoError(t, err)
	res2, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assertMyself(t, res2)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "contacts_test")
	testInstance = setup.GetTestInstance(&lifecycle.Options{
		Email:      "alice@example.com",
		PublicName: "Alice",
	})
	_, token = setup.GetTestClient(consts.Contacts)
	ts = setup.GetTestServer("/contacts", Routes)
	os.Exit(setup.Run())
}
