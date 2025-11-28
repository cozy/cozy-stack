package remote

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemote(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())

	testInstance := setup.GetTestInstance()
	token := generateAppToken(testInstance, "answers", "org.wikidata.entity")

	ts := setup.GetTestServer("/remote", Routes)
	t.Cleanup(ts.Close)

	t.Run("RemoteGET", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.GET("/remote/org.wikidata.entity").
			WithQuery("entity", "Q42").
			WithQuery("comment", "foo").
			WithHeader("Authorization", "Bearer "+token).
			WithHost(testInstance.Domain).
			Expect().Status(200).
			JSON().Object()

		obj.Value("entities").Object().NotEmpty()

		var results []map[string]interface{}
		allReq := &couchdb.AllDocsRequest{
			Descending: true,
			Limit:      1,
		}
		err := couchdb.GetAllDocs(testInstance, consts.RemoteRequests, allReq, &results)
		require.NoError(t, err)
		require.Len(t, results, 1)

		logged := results[0]
		assert.Equal(t, "org.wikidata.entity", logged["doctype"].(string))
		assert.Equal(t, "GET", logged["verb"].(string))
		assert.Equal(t, "https://www.wikidata.org/wiki/Special:EntityData/Q42.json", logged["url"].(string))
		assert.Equal(t, float64(200), logged["response_code"].(float64))
		assert.Equal(t, "application/json", logged["content_type"].(string))
		assert.NotNil(t, logged["created_at"])
		vars := logged["variables"].(map[string]interface{})
		assert.Equal(t, "Q42", vars["entity"].(string))
		assert.Equal(t, "foo", vars["comment"].(string))
		meta, _ := logged["cozyMetadata"].(map[string]interface{})
		assert.Equal(t, "answers", meta["createdByApp"])
	})
}

func generateAppToken(inst *instance.Instance, slug, doctype string) string {
	rules := permission.Set{
		permission.Rule{
			Type:  doctype,
			Verbs: permission.ALL,
		},
	}
	permReq := permission.Permission{
		Permissions: rules,
		Type:        permission.TypeWebapp,
		SourceID:    consts.Apps + "/" + slug,
	}
	err := couchdb.CreateDoc(inst, &permReq)
	if err != nil {
		return ""
	}
	manifest := &couchdb.JSONDoc{
		Type: consts.Apps,
		M: map[string]interface{}{
			"_id":         consts.Apps + "/" + slug,
			"slug":        slug,
			"permissions": rules,
		},
	}
	err = couchdb.CreateNamedDocWithDB(inst, manifest)
	if err != nil {
		return ""
	}
	return inst.BuildAppToken(slug, "")
}

func TestNextcloudDownstreamFailOnConflict(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)

	// Allow loopback addresses for the mock server
	oldBuildMode := build.BuildMode
	build.BuildMode = build.ModeDev
	t.Cleanup(func() {
		build.BuildMode = oldBuildMode
	})

	setup := testutils.NewSetup(t, t.Name())

	testInstance := setup.GetTestInstance()
	token := generateAppToken(testInstance, "testapp", consts.Files)

	// Create a minimal mock NextCloud WebDAV server
	mockWebDAV := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle file download requests (webdav client adds trailing slash)
		if r.Method == "GET" && (r.URL.Path == "/remote.php/dav/files/testuser/testfile.txt/" ||
			r.URL.Path == "/remote.php/dav/files/testuser/testfile2.txt/") {
			content := []byte("downloaded content")
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.WriteHeader(http.StatusOK)
			w.Write(content)
			return
		}
		if r.Method == "DELETE" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// Handle user_status endpoint (needed to get webdav_user_id)
		if r.URL.Path == "/ocs/v2.php/apps/user_status/api/v1/user_status" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ocs":{"data":{"userId":"testuser"}}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockWebDAV.Close()

	ts := setup.GetTestServer("/remote", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(ts.Close)

	e := testutils.CreateTestClient(t, ts.URL)

	// Create a nextcloud account pointing to our mock server
	accountDoc := &couchdb.JSONDoc{
		Type: consts.Accounts,
		M: map[string]interface{}{
			"account_type": "nextcloud",
			"name":         "Test NextCloud",
			"auth": map[string]interface{}{
				"login":    "testuser",
				"password": "testpass",
				"url":      mockWebDAV.URL + "/",
			},
			"webdav_user_id": "testuser",
		},
	}
	account.Encrypt(*accountDoc)
	err := couchdb.CreateDoc(testInstance, accountDoc)
	require.NoError(t, err)
	accountID := accountDoc.ID()

	// Create a file in the VFS that will conflict
	fs := testInstance.VFS()
	dirDoc, err := vfs.NewDirDoc(fs, "testdir", "", nil)
	require.NoError(t, err)
	err = fs.CreateDir(dirDoc)
	require.NoError(t, err)

	t.Run("DownstreamWithoutFailOnConflict", func(t *testing.T) {
		// First, create a file that will conflict
		existingContent := []byte("existing")
		conflictFile, err := vfs.NewFileDoc(
			"testfile.txt",
			dirDoc.ID(),
			int64(len(existingContent)),
			nil,
			"text/plain",
			"",
			time.Now(),
			false,
			false,
			false,
			nil,
		)
		require.NoError(t, err)
		file, err := fs.CreateFile(conflictFile, nil)
		require.NoError(t, err)
		_, err = file.Write([]byte("existing"))
		require.NoError(t, err)
		err = file.Close()
		require.NoError(t, err)

		// Without FailOnConflict, it should auto-rename and succeed
		res := e.POST("/remote/nextcloud/"+accountID+"/downstream/testfile.txt").
			WithQuery("To", dirDoc.ID()).
			WithQuery("FailOnConflict", "false").
			WithHeader("Authorization", "Bearer "+token).
			WithHost(testInstance.Domain).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		// The file should be created with an auto-generated name
		attrs := res.Path("$.data.attributes").Object()
		name := attrs.Value("name").String().Raw()
		assert.Contains(t, name, "testfile")
		assert.NotEqual(t, "testfile.txt", name, "File should be renamed due to conflict")
	})

	t.Run("DownstreamWithFailOnConflict", func(t *testing.T) {
		// Create a file that will conflict (use a different name to avoid leftover from previous test)
		existingContent := []byte("existing")
		conflictFile, err := vfs.NewFileDoc(
			"testfile2.txt",
			dirDoc.ID(),
			int64(len(existingContent)),
			nil,
			"text/plain",
			"",
			time.Now(),
			false,
			false,
			false,
			nil,
		)
		require.NoError(t, err)
		file, err := fs.CreateFile(conflictFile, nil)
		require.NoError(t, err)
		_, err = file.Write(existingContent)
		require.NoError(t, err)
		err = file.Close()
		require.NoError(t, err)

		// With FailOnConflict=true, it should return 409 Conflict
		e.POST("/remote/nextcloud/"+accountID+"/downstream/testfile2.txt").
			WithQuery("To", dirDoc.ID()).
			WithQuery("FailOnConflict", "true").
			WithHeader("Authorization", "Bearer "+token).
			WithHost(testInstance.Domain).
			Expect().Status(409)
	})
}
