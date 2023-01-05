package office

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
	"github.com/stretchr/testify/require"
)

var ts *httptest.Server
var inst *instance.Instance
var ooURL string
var token string
var fileID string
var key string

type fakeServer struct {
	count int
}

func TestOffice(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile()
	ooURL = fakeOOServer()
	config.GetConfig().Office = map[string]config.Office{
		"default": {OnlyOfficeURL: ooURL},
	}
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(nil, t.Name())
	t.Cleanup(setup.Cleanup)
	inst = setup.GetTestInstance()
	_, token = setup.GetTestClient(consts.Files)

	if err := createFile(); err != nil {
		os.Exit(1)
	}

	ts = setup.GetTestServer("/office", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler

	t.Run("OnlyOfficeLocal", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ts.URL+"/office/"+fileID+"/open", nil)
		req.Header.Add("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		require.Equal(t, 200, res.StatusCode)

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
		oo, _ := attrs["onlyoffice"].(map[string]interface{})
		assert.NotEmpty(t, oo["url"])
		assert.Equal(t, "word", oo["documentType"])
		editor, _ := oo["editor"].(map[string]interface{})
		assert.Equal(t, "edit", editor["mode"])
		callbackURL, _ := editor["callbackUrl"].(string)
		assert.True(t, strings.HasSuffix(callbackURL, "/office/callback"))
		document, _ := oo["document"].(map[string]interface{})
		key, _ = document["key"].(string)
		assert.NotEmpty(t, key)
	})

	t.Run("SaveOnlyOffice", func(t *testing.T) {
		// Force save
		body := fmt.Sprintf(`{
		"actions": [{"type": 0, "userid": "78e1e841"}],
		"key": "%s",
		"status": 6,
		"url": "%s",
		"users": ["6d5a81d0"]
	}`, key, ooURL+"/dl")
		req, _ := http.NewRequest("POST", ts.URL+"/office/callback", strings.NewReader(body))
		req.Header.Add("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		require.Equal(t, 200, res.StatusCode)

		var data map[string]interface{}
		err = json.NewDecoder(res.Body).Decode(&data)
		assert.NoError(t, err)
		assert.Equal(t, data["error"], 0.0)

		doc, err := inst.VFS().FileByID(fileID)
		assert.NoError(t, err)
		file, err := inst.VFS().OpenFile(doc)
		assert.NoError(t, err)
		defer file.Close()
		buf, err := io.ReadAll(file)
		assert.NoError(t, err)
		assert.Equal(t, "version 1", string(buf))

		// Final save
		body = strings.Replace(body, `"status": 6`, `"status": 2`, 1)
		req, _ = http.NewRequest("POST", ts.URL+"/office/callback", strings.NewReader(body))
		req.Header.Add("Content-Type", "application/json")
		res, err = http.DefaultClient.Do(req)
		assert.NoError(t, err)
		require.Equal(t, 200, res.StatusCode)

		defer res.Body.Close()

		doc, err = inst.VFS().FileByID(fileID)
		assert.NoError(t, err)
		file, err = inst.VFS().OpenFile(doc)
		assert.NoError(t, err)
		defer file.Close()
		buf, err = io.ReadAll(file)
		assert.NoError(t, err)
		assert.Equal(t, "version 2", string(buf))
	})
}

func createFile() error {
	dirID := consts.RootDirID
	filedoc, err := vfs.NewFileDoc("letter.docx", dirID, -1, nil,
		"application/msword", "text", time.Now(), false, false, false, nil)
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

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.count++
	body := fmt.Sprintf("version %d", f.count)
	_, _ = w.Write([]byte(body))
}

func fakeOOServer() string {
	handler := &fakeServer{}
	server := httptest.NewServer(handler)
	return server.URL
}
