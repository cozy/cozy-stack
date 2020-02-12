package shortcuts

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

var ts *httptest.Server
var inst *instance.Instance
var token string
var shortcutID string

func TestCreateShortcut(t *testing.T) {
	body := `
{
  "data": {
    "type": "io.cozy.files.shortcuts",
    "attributes": {
      "name": "sunset.jpg.url",
      "url": "https://alice-photos.cozy.example/#/photos/629fb233be550a21174ac8e19f0043af",
      "metadata": {
        "target": {
          "cozyMetadata": {
            "instance": "https://alice.cozy.example/"
          },
          "app": "photos",
          "_type": "io.cozy.files",
          "mime": "image/jpg"
        }
      }
    }
  }
}`
	req, _ := http.NewRequest("POST", ts.URL+"/shortcuts", bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 201, res.StatusCode)
	var result map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)

	data, _ := result["data"].(map[string]interface{})
	assert.Equal(t, "io.cozy.files", data["type"])
	assert.Contains(t, data, "id")
	shortcutID = data["id"].(string)
	attrs := data["attributes"].(map[string]interface{})
	assert.Equal(t, "file", attrs["type"])
	assert.Equal(t, "sunset.jpg.url", attrs["name"])
	assert.Equal(t, "application/internet-shortcut", attrs["mime"])
	fcm, _ := attrs["cozyMetadata"].(map[string]interface{})
	assert.Contains(t, fcm, "createdAt")
	assert.Contains(t, fcm, "createdOn")
	meta, _ := attrs["metadata"].(map[string]interface{})
	target, _ := meta["target"].(map[string]interface{})
	assert.Equal(t, "photos", target["app"])
	assert.Equal(t, "io.cozy.files", target["_type"])
	assert.Equal(t, "image/jpg", target["mime"])
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "shortcuts_test")
	inst = setup.GetTestInstance()
	_, token = setup.GetTestClient(consts.Files)

	ts = setup.GetTestServer("/shortcuts", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	os.Exit(setup.Run())
}
