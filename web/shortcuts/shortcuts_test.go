package shortcuts

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/tests/testutils"
	weberrors "github.com/cozy/cozy-stack/web/errors"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

var ts *httptest.Server
var inst *instance.Instance
var token string
var shortcutID string

const targetURL = "https://alice-photos.cozy.example/#/photos/629fb233be550a21174ac8e19f0043af"

func TestCreateShortcut(t *testing.T) {
	body := `
{
  "data": {
    "type": "io.cozy.files.shortcuts",
    "attributes": {
      "name": "sunset.jpg.url",
      "url": "` + targetURL + `",
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

func TestGetShortcut(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/shortcuts/"+shortcutID, nil)
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("Accept", "application/vnd.api+json")
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	var result map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)

	data, _ := result["data"].(map[string]interface{})
	assert.Equal(t, "io.cozy.files.shortcuts", data["type"])
	assert.Equal(t, shortcutID, data["id"])
	shortcutID = data["id"].(string)
	attrs := data["attributes"].(map[string]interface{})
	assert.Equal(t, "sunset.jpg.url", attrs["name"])
	assert.Equal(t, "io.cozy.files.root-dir", attrs["dir_id"])
	assert.Equal(t, targetURL, attrs["url"])
	meta := attrs["metadata"].(map[string]interface{})
	target := meta["target"].(map[string]interface{})
	assert.Equal(t, "photos", target["app"])
	assert.Equal(t, "io.cozy.files", target["_type"])
	assert.Equal(t, "image/jpg", target["mime"])

	client := http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return errors.New("Do not follow the redirections")
		},
	}
	req, _ = http.NewRequest("GET", ts.URL+"/shortcuts/"+shortcutID, nil)
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("Accept", "text/html")
	res, err = client.Do(req)
	assert.Contains(t, err.Error(), "Do not follow the redirections")
	assert.Equal(t, 303, res.StatusCode)
	assert.Equal(t, targetURL, res.Header.Get("Location"))
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "shortcuts_test")
	inst = setup.GetTestInstance()
	_, token = setup.GetTestClient(consts.Files)

	ts = setup.GetTestServer("/shortcuts", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = weberrors.ErrorHandler
	os.Exit(setup.Run())
}
