package office

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeServer struct {
	count int
}

func TestOffice(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	var key string

	config.UseTestFile(t)
	ooURL := fakeOOServer()
	config.GetConfig().Office = map[string]config.Office{
		"default": {OnlyOfficeURL: ooURL},
	}
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	inst := setup.GetTestInstance()
	_, token := setup.GetTestClient(consts.Files)

	fileID := createFile(t, inst)

	ts := setup.GetTestServer("/office", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(ts.Close)

	t.Run("OnlyOfficeLocal", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.GET("/office/"+fileID+"/open").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		data := obj.Value("data").Object()
		data.ValueEqual("type", consts.OfficeURL)
		data.ValueEqual("id", fileID)

		attrs := data.Value("attributes").Object()
		attrs.ValueEqual("document_id", fileID)
		attrs.ValueEqual("subdomain", "nested")
		attrs.Value("protocol").String().Contains("http")
		attrs.ValueEqual("instance", inst.Domain)
		attrs.Value("public_name").String().NotEmpty()

		oo := attrs.Value("onlyoffice").Object()
		oo.Value("url").String().NotEmpty()
		oo.ValueEqual("documentType", "word")

		editor := oo.Value("editor").Object()
		editor.ValueEqual("mode", "edit")
		editor.Value("callbackUrl").String().HasSuffix("/office/callback")

		document := oo.Value("document").Object()
		key = document.Value("key").String().NotEmpty().Raw()
	})

	t.Run("SaveOnlyOffice", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Force save
		obj := e.POST("/office/callback").
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(fmt.Sprintf(`{
      "actions": [{"type": 0, "userid": "78e1e841"}],
      "key": "%s",
      "status": 6,
      "url": "%s",
      "users": ["6d5a81d0"]
    }`, key, ooURL+"/dl"))).
			Expect().Status(200).
			JSON().Object()

		obj.ValueEqual("error", 0.0)

		doc, err := inst.VFS().FileByID(fileID)
		assert.NoError(t, err)
		file, err := inst.VFS().OpenFile(doc)
		assert.NoError(t, err)
		defer file.Close()
		buf, err := io.ReadAll(file)
		assert.NoError(t, err)
		assert.Equal(t, "version 1", string(buf))

		// Final save
		e.POST("/office/callback").
			WithHeader("Content-Type", "application/json").
			// Change "status": 6 -> "status": 2
			WithBytes([]byte(fmt.Sprintf(`{
      "actions": [{"type": 0, "userid": "78e1e841"}],
      "key": "%s",
      "status": 2,
      "url": "%s",
      "users": ["6d5a81d0"]
    }`, key, ooURL+"/dl"))).
			Expect().Status(200)

		doc, err = inst.VFS().FileByID(fileID)
		assert.NoError(t, err)
		file, err = inst.VFS().OpenFile(doc)
		assert.NoError(t, err)
		defer file.Close()
		buf, err = io.ReadAll(file)
		assert.NoError(t, err)
		assert.Equal(t, "version 2", string(buf))
	})

	t.Run("Conflict after an upload", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// When a user opens an office document
		obj := e.GET("/office/"+fileID+"/open").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()
		data := obj.Value("data").Object()
		attrs := data.Value("attributes").Object()
		oo := attrs.Value("onlyoffice").Object()
		document := oo.Value("document").Object()
		key = document.Value("key").String().NotEmpty().Raw()

		// the key is associated to this file
		obj = e.POST("/office/keys/"+key).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()
		data = obj.Value("data").Object()
		docID := data.Value("id").String().NotEmpty().Raw()
		assert.Equal(t, fileID, docID)
		attrs = data.Value("attributes").Object()
		name := attrs.Value("name").String().NotEmpty().Raw()
		assert.Equal(t, "letter.docx", name)

		// When an upload is made that changes the content of this document,
		// the key will now be associated to a conflict file
		updateFile(t, inst, fileID)
		obj = e.POST("/office/keys/"+key).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()
		data = obj.Value("data").Object()
		conflictID := data.Value("id").String().NotEmpty().Raw()
		assert.NotEqual(t, fileID, conflictID)
		meta := data.Value("meta").Object()
		conflictRev := meta.Value("rev").String().NotEmpty().Raw()
		attrs = data.Value("attributes").Object()
		conflictName := attrs.Value("name").String().NotEmpty().Raw()
		assert.Equal(t, "letter (2).docx", conflictName)

		// When another user uses the same key, they obtains the same file
		obj = e.POST("/office/keys/"+key).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()
		data = obj.Value("data").Object()
		anotherID := data.Value("id").String().NotEmpty().Raw()
		assert.Equal(t, conflictID, anotherID)
		meta = data.Value("meta").Object()
		anotherRev := meta.Value("rev").String().NotEmpty().Raw()
		assert.Equal(t, conflictRev, anotherRev)
		attrs = data.Value("attributes").Object()
		anotherName := attrs.Value("name").String().NotEmpty().Raw()
		assert.Equal(t, conflictName, anotherName)

		// When another user opens the document, a new key is given
		obj = e.GET("/office/"+fileID+"/open").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()
		data = obj.Value("data").Object()
		attrs = data.Value("attributes").Object()
		oo = attrs.Value("onlyoffice").Object()
		document = oo.Value("document").Object()
		newkey := document.Value("key").String().NotEmpty().Raw()
		assert.NotEqual(t, key, newkey)

		// When the document is saved with the first key, it's written to the
		// conflict file
		e.POST("/office/callback").
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(fmt.Sprintf(`{
      "actions": [{"type": 0, "userid": "78e1e841"}],
      "key": "%s",
      "status": 2,
      "url": "%s",
      "users": ["6d5a81d0"]
    }`, key, ooURL+"/dl"))).
			Expect().Status(200)
		conflict, err := inst.VFS().FileByID(conflictID)
		require.NoError(t, err)
		assert.Equal(t, "letter (2).docx", conflict.DocName)
		assert.Equal(t, "onlyoffice-server", conflict.CozyMetadata.UploadedBy.Slug)
		assert.Equal(t, "onlyoffice-server", conflict.CozyMetadata.UpdatedByApps[0].Slug)
		assert.NotEqual(t, conflictRev, conflict.Rev())
	})
}

func createFile(t *testing.T, inst *instance.Instance) string {
	dirID := consts.RootDirID
	filedoc, err := vfs.NewFileDoc("letter.docx", dirID, -1, nil,
		"application/msword", "text", time.Now(), false, false, false, nil)
	require.NoError(t, err)

	f, err := inst.VFS().CreateFile(filedoc, nil)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	return filedoc.ID()
}

func updateFile(t *testing.T, inst *instance.Instance, fileID string) {
	olddoc, err := inst.VFS().FileByID(fileID)
	require.NoError(t, err)

	newdoc := olddoc.Clone().(*vfs.FileDoc)
	newdoc.ByteSize = -1
	newdoc.MD5Sum = nil
	newdoc.CozyMetadata.UploadedBy = &vfs.UploadedByEntry{
		Slug:    "desktop",
		Version: "0.0.1",
	}

	f, err := inst.VFS().CreateFile(newdoc, olddoc)
	require.NoError(t, err)
	_, err = io.WriteString(f, "updated")
	require.NoError(t, err)
	require.NoError(t, f.Close())
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
