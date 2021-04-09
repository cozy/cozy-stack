package office

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
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
var fileID string

func TestOpenOfficeLocal(t *testing.T) {
	req, _ := http.NewRequest("GET", ts.URL+"/office/"+fileID+"/open", nil)
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	if !assert.Equal(t, 200, res.StatusCode) {
		return
	}
	var doc map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&doc)
	assert.NoError(t, err)
	data, _ := doc["data"].(map[string]interface{})
	assert.Equal(t, consts.OfficeURL, data["type"])
	assert.Equal(t, fileID, data["id"])
	attrs, _ := data["attributes"].(map[string]interface{})
	assert.Equal(t, fileID, attrs["document_id"])
	assert.Equal(t, "nested", attrs["subdomain"])
	assert.Contains(t, attrs["protocol"], "http")
	assert.Equal(t, inst.Domain, attrs["instance"])
	assert.NotEmpty(t, attrs["public_name"])
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	config.GetConfig().Office = map[string]config.Office{
		"default": {OnlyOfficeURL: "http://localhost:9000/"},
	}
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "notes_test")
	inst = setup.GetTestInstance()
	_, token = setup.GetTestClient(consts.Files)

	if err := createFile(); err != nil {
		fmt.Printf("Could not create office doc: %s\n", err)
		os.Exit(1)
	}

	ts = setup.GetTestServer("/office", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	os.Exit(setup.Run())
}

func createFile() error {
	dirID := consts.RootDirID
	filedoc, err := vfs.NewFileDoc("letter.docx", dirID, -1, nil,
		"application/msword", "text", time.Now(), false, false, nil)
	if err != nil {
		return err
	}
	f, err := inst.VFS().CreateFile(filedoc, nil)
	if err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	fileID = filedoc.ID()
	return nil
}
