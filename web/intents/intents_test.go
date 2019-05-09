package intents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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
	"github.com/cozy/echo"
	"github.com/stretchr/testify/assert"
)

var ts *httptest.Server
var ins *instance.Instance
var token string
var appToken string
var filesToken string
var intentID string
var appPerms *permission.Permission

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
	checkIntentResult(t, res, true)
}

func TestGetIntent(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/intents/"+intentID, nil)
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add("Accept", "application/vnd.api+json")
	req.Header.Add("Authorization", "Bearer "+filesToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	checkIntentResult(t, res, true)
}

func TestGetIntentNotFromTheService(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/intents/"+intentID, nil)
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add("Accept", "application/vnd.api+json")
	req.Header.Add("Authorization", "Bearer "+appToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 403, res.StatusCode)
}

func TestCreateIntentOAuth(t *testing.T) {
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
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "intents_test")
	ins = setup.GetTestInstance(&lifecycle.Options{
		Domain: "cozy.example.net",
	})
	_, token = setup.GetTestClient(consts.Settings)

	webapp := &app.WebappManifest{
		DocID:          consts.Apps + "/app",
		DocSlug:        "app",
		DocPermissions: permission.Set{},
	}
	err := couchdb.CreateNamedDoc(ins, webapp)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	appPerms, err = permission.CreateWebappSet(ins, webapp.Slug(), webapp.Permissions())
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	appToken = ins.BuildAppToken(webapp.Slug(), "")
	files := &app.WebappManifest{
		DocID:          consts.Apps + "/files",
		DocSlug:        "files",
		DocPermissions: permission.Set{},
		Intents: []app.Intent{
			{
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
	if _, err := permission.CreateWebappSet(ins, files.Slug(), files.Permissions()); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	filesToken = ins.BuildAppToken(files.Slug(), "")

	ts = setup.GetTestServer("/intents", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	os.Exit(setup.Run())

}
