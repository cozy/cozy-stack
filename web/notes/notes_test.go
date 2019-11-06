package notes

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
	"github.com/stretchr/testify/assert"
)

var ts *httptest.Server
var inst *instance.Instance
var token string

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
	data, _ := result["data"].(map[string]interface{})
	assert.Equal(t, "io.cozy.files", data["type"])
	assert.Contains(t, data, "id")
	attrs := data["attributes"].(map[string]interface{})
	assert.Equal(t, "file", attrs["type"])
	assert.Equal(t, "A super note.cozy-note", attrs["name"])
	fcm, _ := attrs["cozyMetadata"].(map[string]interface{})
	assert.Contains(t, fcm, "createdAt")
	assert.Contains(t, fcm, "createdOn")
	meta, _ := attrs["metadata"].(map[string]interface{})
	assert.Equal(t, "A super note", meta["title"])
	assert.EqualValues(t, 0, meta["revision"])
	assert.Contains(t, meta, "schema")
	assert.Contains(t, meta, "content")
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "notes_test")
	inst = setup.GetTestInstance()
	_, token = setup.GetTestClient(consts.Files)

	ts = setup.GetTestServer("/notes", Routes)
	os.Exit(setup.Run())
}
