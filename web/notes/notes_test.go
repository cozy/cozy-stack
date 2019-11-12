package notes

import (
	"bytes"
	"encoding/json"
	"fmt"
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
var noteID string

func TestCreateNote(t *testing.T) {
	body := `
{
  "data": {
    "type": "io.cozy.notes.documents",
    "attributes": {
      "title": "A super note",
      "schema": {
        "nodes": [
          ["doc", { "content": "block+" }],
          ["paragraph", { "content": "inline*", "group": "block" }],
          ["blockquote", { "content": "block+", "group": "block" }],
          ["horizontal_rule", { "group": "block" }],
          [
            "heading",
            {
              "content": "inline*",
              "group": "block",
              "attrs": { "level": { "default": 1 } }
            }
          ],
          ["code_block", { "content": "text*", "marks": "", "group": "block" }],
          ["text", { "group": "inline" }],
          [
            "image",
            {
              "group": "inline",
              "inline": true,
              "attrs": { "alt": {}, "src": {}, "title": {} }
            }
          ],
          ["hard_break", { "group": "inline", "inline": true }],
          [
            "ordered_list",
            {
              "content": "list_item+",
              "group": "block",
              "attrs": { "order": { "default": 1 } }
            }
          ],
          ["bullet_list", { "content": "list_item+", "group": "block" }],
          ["list_item", { "content": "paragraph block*" }]
        ],
        "marks": [
          ["link", { "attrs": { "href": {}, "title": {} }, "inclusive": false }],
          ["em", {}],
          ["strong", {}],
          ["code", {}]
        ],
        "topNode": "doc"
      }
    }
  }
}`
	req, _ := http.NewRequest("POST", ts.URL+"/notes", bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 201, res.StatusCode)
	var result map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)
	assertInitialNote(t, result)
}

func TestGetNote(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/notes/"+noteID, nil)
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	var result map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)
	assertInitialNote(t, result)
}

func assertInitialNote(t *testing.T, result map[string]interface{}) {
	data, _ := result["data"].(map[string]interface{})
	assert.Equal(t, "io.cozy.files", data["type"])
	if noteID == "" {
		assert.Contains(t, data, "id")
		noteID = data["id"].(string)
	} else {
		assert.Equal(t, noteID, data["id"])
	}
	attrs := data["attributes"].(map[string]interface{})
	assert.Equal(t, "file", attrs["type"])
	assert.Equal(t, "A super note.cozy-note", attrs["name"])
	fcm, _ := attrs["cozyMetadata"].(map[string]interface{})
	assert.Contains(t, fcm, "createdAt")
	assert.Contains(t, fcm, "createdOn")
	meta, _ := attrs["metadata"].(map[string]interface{})
	assert.Equal(t, "A super note", meta["title"])
	assert.EqualValues(t, 0, meta["revision"])
	assert.NotNil(t, meta["schema"])
	assert.NotNil(t, meta["content"])
}

func TestChangeTitle(t *testing.T) {
	body := `
{
  "data": {
    "type": "io.cozy.notes.documents",
    "attributes": {
      "title": "A new title"
    }
  }
}`
	req, _ := http.NewRequest("PUT", ts.URL+"/notes/"+noteID+"/title", bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	var result map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)

	data, _ := result["data"].(map[string]interface{})
	assert.Equal(t, "io.cozy.files", data["type"])
	assert.Equal(t, noteID, data["id"])
	attrs := data["attributes"].(map[string]interface{})
	assert.Equal(t, "A new title.cozy-note", attrs["name"])
	meta, _ := attrs["metadata"].(map[string]interface{})
	assert.Equal(t, "A new title", meta["title"])
	assert.EqualValues(t, 0, meta["revision"])
	assert.NotNil(t, meta["schema"])
	assert.NotNil(t, meta["content"])
}

func TestPatchNote(t *testing.T) {
	body := `{
  "data": [{
    "type": "io.cozy.notes.steps",
    "attributes": {
      "stepType": "replace",
      "from": 1,
      "to": 1,
      "slice": {
        "content": [{ "type": "text", "text": "H" }]
      }
    }
  }, {
    "type": "io.cozy.notes.steps",
    "attributes": {
      "stepType": "replace",
      "from": 2,
      "to": 2,
      "slice": {
        "content": [{ "type": "text", "text": "ello" }]
      }
    }
  }]
}`
	req, _ := http.NewRequest("PATCH", ts.URL+"/notes/"+noteID, bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("If-Match", "0")
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	var result map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&result)
	assert.NoError(t, err)
	if res.StatusCode != 200 {
		fmt.Printf("result = %#v\n", result)
		return
	}

	data, _ := result["data"].(map[string]interface{})
	assert.Equal(t, "io.cozy.files", data["type"])
	assert.Equal(t, noteID, data["id"])
	attrs := data["attributes"].(map[string]interface{})
	meta, _ := attrs["metadata"].(map[string]interface{})
	assert.EqualValues(t, 2, meta["revision"])
	assert.NotNil(t, meta["schema"])
	assert.NotNil(t, meta["content"])
	fmt.Printf("content = %#v\n\n", meta["content"])

	req, _ = http.NewRequest("PATCH", ts.URL+"/notes/"+noteID, bytes.NewBufferString(body))
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("If-Match", "0")
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 409, res.StatusCode)

	var result2 map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&result2)
	assert.NoError(t, err)
	fmt.Printf("result = %#v\n", result2)
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "notes_test")
	inst = setup.GetTestInstance()
	_, token = setup.GetTestClient(consts.Files)

	ts = setup.GetTestServer("/notes", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	os.Exit(setup.Run())
}
