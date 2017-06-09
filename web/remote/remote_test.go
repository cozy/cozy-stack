package remote

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
)

var testInstance *instance.Instance
var ts *httptest.Server
var token string

func TestRemoteGET(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/remote/org.wikidata.entity?entity=Q42", nil)
	req.Header.Add("Authorization", "Bearer "+token)
	req.Host = testInstance.Domain
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	body, _ := ioutil.ReadAll(res.Body)
	assert.Equal(t, `{"entities":`, string(body[:12]))
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "remote_test")

	testInstance = setup.GetTestInstance()
	_, token = setup.GetTestClient("org.wikidata.entity")

	ts = setup.GetTestServer("/remote", Routes)
	os.Exit(setup.Run())
}
