package intents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
)

var ts *httptest.Server
var ins *instance.Instance
var token string
var appToken string
var filesToken string
var intentID string

func TestCreateIntent(t *testing.T) {
	body := `{
		"data": {
			"type": "io.cozy.settings",
			"attributes": {
				"action": "PICK",
				"type": "io.cozy.files",
				"permissions": ["GET"]
			}
		}
	}`
	req, _ := http.NewRequest("POST", ts.URL+"/intents", bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add("Accept", "application/vnd.api+json")
	req.Header.Add("Authorization", "Bearer "+appToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)
	data, ok := result["data"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "io.cozy.intents", data["type"].(string))
	intentID = data["id"].(string)
	assert.NotEmpty(t, intentID)
	attrs := data["attributes"].(map[string]interface{})
	assert.Equal(t, "PICK", attrs["action"].(string))
	assert.Equal(t, "io.cozy.files", attrs["type"].(string))
	assert.Equal(t, "app.cozy.example.net", attrs["client"].(string))
	perms := attrs["permissions"].([]interface{})
	assert.Len(t, perms, 1)
	assert.Equal(t, "GET", perms[0].(string))
}

func TestCreateIntentIsRejectedForOAuthClients(t *testing.T) {
	body := `{
		"data": {
			"type": "io.cozy.settings",
			"attributes": {
				"action": "PICK",
				"type": "io.cozy.files",
				"permissions": ["GET"]
			}
		}
	}`
	req, _ := http.NewRequest("POST", ts.URL+"/intents", bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add("Accept", "application/vnd.api+json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, res.StatusCode)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "intents_test")
	ins = setup.GetTestInstance(&instance.Options{
		Domain: "cozy.example.net",
	})
	_, token = setup.GetTestClient(consts.Settings)

	app := &apps.Manifest{
		Slug:        "app",
		Permissions: &permissions.Set{},
	}
	if err := couchdb.CreateNamedDoc(ins, app); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if _, err := permissions.CreateAppSet(ins, app.Slug, *app.Permissions); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	appToken = ins.BuildAppToken(app)
	files := &apps.Manifest{
		Slug:        "files",
		Permissions: &permissions.Set{},
		Intents: []apps.Intent{
			apps.Intent{
				Action: "PICK",
				Types:  []string{"io.cozy.files", "image/gif"},
				Href:   "/pick",
			},
		},
	}
	if err := couchdb.CreateNamedDoc(ins, files); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if _, err := permissions.CreateAppSet(ins, files.Slug, *files.Permissions); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	filesToken = ins.BuildAppToken(app)

	ts = setup.GetTestServer("/intents", Routes)
	os.Exit(setup.Run())

}
