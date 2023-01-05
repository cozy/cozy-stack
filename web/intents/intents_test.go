package intents

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ts *httptest.Server
var ins *instance.Instance
var token string
var appToken string
var filesToken string
var intentID string
var appPerms *permission.Permission

func TestIntents(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	var err error

	config.UseTestFile()
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(nil, t.Name())
	t.Cleanup(setup.Cleanup)
	ins = setup.GetTestInstance(&lifecycle.Options{
		Domain: "cozy.example.net",
	})
	_, token = setup.GetTestClient(consts.Settings)

	webapp := &couchdb.JSONDoc{
		Type: consts.Apps,
		M: map[string]interface{}{
			"_id":  consts.Apps + "/app",
			"slug": "app",
		},
	}
	require.NoError(t, couchdb.CreateNamedDoc(ins, webapp))

	appPerms, err = permission.CreateWebappSet(ins, "app", permission.Set{}, "1.0.0")
	if err != nil {
		require.NoError(t, err)
	}
	appToken = ins.BuildAppToken("app", "")
	files := &couchdb.JSONDoc{
		Type: consts.Apps,
		M: map[string]interface{}{
			"_id":  consts.Apps + "/files",
			"slug": "files",
			"intents": []app.Intent{
				{
					Action: "PICK",
					Types:  []string{"io.cozy.files", "image/gif"},
					Href:   "/pick",
				},
			},
		},
	}

	require.NoError(t, couchdb.CreateNamedDoc(ins, files))
	if _, err := permission.CreateWebappSet(ins, "files", permission.Set{}, "1.0.0"); err != nil {
		require.NoError(t, err)
	}
	filesToken = ins.BuildAppToken("files", "")

	ts = setup.GetTestServer("/intents", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler

	t.Run("CreateIntent", func(t *testing.T) {
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
		checkIntentResult(t, res, true)
	})

	t.Run("GetIntent", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/intents/"+intentID, nil)
		req.Header.Add("Content-Type", "application/vnd.api+json")
		req.Header.Add("Accept", "application/vnd.api+json")
		req.Header.Add("Authorization", "Bearer "+filesToken)
		res, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		checkIntentResult(t, res, true)
	})

	t.Run("GetIntentNotFromTheService", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/intents/"+intentID, nil)
		req.Header.Add("Content-Type", "application/vnd.api+json")
		req.Header.Add("Accept", "application/vnd.api+json")
		req.Header.Add("Authorization", "Bearer "+appToken)
		res, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, 403, res.StatusCode)
	})

	t.Run("CreateIntentOAuth", func(t *testing.T) {
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
		checkIntentResult(t, res, false)
	})
}

func checkIntentResult(t *testing.T, res *http.Response, fromWeb bool) {
	assert.Equal(t, 200, res.StatusCode)
	var result map[string]interface{}
	err := json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)
	data, ok := result["data"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "io.cozy.intents", data["type"].(string))
	intentID = data["id"].(string)
	assert.NotEmpty(t, intentID)
	attrs := data["attributes"].(map[string]interface{})
	perms := attrs["permissions"].([]interface{})
	assert.Len(t, perms, 1)
	assert.Equal(t, "GET", perms[0].(string))
	assert.Equal(t, "PICK", attrs["action"].(string))
	assert.Equal(t, "io.cozy.files", attrs["type"].(string))
	if !fromWeb {
		return
	}
	assert.Equal(t, "https://app.cozy.example.net", attrs["client"].(string))
	links := data["links"].(map[string]interface{})
	assert.Equal(t, "/intents/"+intentID, links["self"].(string))
	assert.Equal(t, "/permissions/"+appPerms.ID(), links["permissions"].(string))
}
