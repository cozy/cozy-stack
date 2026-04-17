package remote

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

func TestNextcloudSize(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)

	oldBuildMode := build.BuildMode
	build.BuildMode = build.ModeDev
	t.Cleanup(func() { build.BuildMode = oldBuildMode })

	setup := testutils.NewSetup(t, t.Name())
	testInstance := setup.GetTestInstance()
	token := generateAppToken(testInstance, "testapp", consts.Files)

	// Mock Nextcloud: answer the OCS probe for the account constructor,
	// and a Depth:0 PROPFIND that asks for oc:size with a multistatus
	// reply carrying a hard-coded byte total. The test asserts the stack
	// parses and surfaces that number verbatim.
	const wantSize uint64 = 67365343
	var lastPropfindPath string
	mockWebDAV := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ocs/v2.php/cloud/user" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ocs":{"data":{"id":"testuser"}}}`))
			return
		}
		if r.Method == "PROPFIND" && strings.HasPrefix(r.URL.Path, "/remote.php/dav/files/testuser/") {
			lastPropfindPath = r.URL.Path
			w.Header().Set("Content-Type", "application/xml; charset=utf-8")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write([]byte(`<?xml version="1.0"?>
<d:multistatus xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns">
  <d:response>
    <d:href>` + r.URL.Path + `</d:href>
    <d:propstat>
      <d:prop><oc:size>67365343</oc:size></d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(mockWebDAV.Close)

	ts := setup.GetTestServer("/remote", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(ts.Close)

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
	require.NoError(t, couchdb.CreateDoc(testInstance, accountDoc))
	accountID := accountDoc.ID()

	e := testutils.CreateTestClient(t, ts.URL)

	t.Run("ReturnsTheRecursiveSizeOfASubfolder", func(t *testing.T) {
		obj := e.GET("/remote/nextcloud/"+accountID+"/size/Photos").
			WithHeader("Authorization", "Bearer "+token).
			WithHost(testInstance.Domain).
			Expect().Status(http.StatusOK).
			JSON().Object()

		obj.Value("size").Number().IsEqual(wantSize)
		assert.Equal(t, "/remote.php/dav/files/testuser/Photos/", lastPropfindPath,
			"should PROPFIND the requested sub-path")
	})

	t.Run("TreatsAnEmptyPathAsTheAccountRoot", func(t *testing.T) {
		lastPropfindPath = ""
		obj := e.GET("/remote/nextcloud/"+accountID+"/size/").
			WithHeader("Authorization", "Bearer "+token).
			WithHost(testInstance.Domain).
			Expect().Status(http.StatusOK).
			JSON().Object()

		obj.Value("size").Number().IsEqual(wantSize)
		assert.Equal(t, "/remote.php/dav/files/testuser/", lastPropfindPath,
			"should PROPFIND the account root for an empty sub-path")
	})

	t.Run("Returns404WhenTheAccountDoesNotExist", func(t *testing.T) {
		e.GET("/remote/nextcloud/does-not-exist/size/Photos").
			WithHeader("Authorization", "Bearer "+token).
			WithHost(testInstance.Domain).
			Expect().Status(http.StatusNotFound)
	})
}

func TestNextcloudSizeErrorClassification(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)

	oldBuildMode := build.BuildMode
	build.BuildMode = build.ModeDev
	t.Cleanup(func() { build.BuildMode = oldBuildMode })

	setup := testutils.NewSetup(t, t.Name())
	testInstance := setup.GetTestInstance()
	token := generateAppToken(testInstance, "testapp", consts.Files)

	// Mock Nextcloud that always rejects credentials on PROPFIND so the
	// stack's error-wrapping layer has to pick a mapping. Anything other
	// than 401 would mean wrapNextcloudErrors missed a branch.
	mockWebDAV := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ocs/v2.php/cloud/user" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ocs":{"data":{"id":"testuser"}}}`))
			return
		}
		if r.Method == "PROPFIND" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(mockWebDAV.Close)

	ts := setup.GetTestServer("/remote", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(ts.Close)

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
	require.NoError(t, couchdb.CreateDoc(testInstance, accountDoc))
	accountID := accountDoc.ID()

	e := testutils.CreateTestClient(t, ts.URL)

	t.Run("Surfaces401WhenNextcloudRejectsTheCredentials", func(t *testing.T) {
		e.GET("/remote/nextcloud/"+accountID+"/size/Photos").
			WithHeader("Authorization", "Bearer "+token).
			WithHost(testInstance.Domain).
			Expect().Status(http.StatusUnauthorized)
	})
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
		// Handle OCS cloud/user endpoint (needed to get webdav_user_id)
		if r.URL.Path == "/ocs/v2.php/cloud/user" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ocs":{"data":{"id":"testuser"}}}`))
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

// TestNextcloudPathParam pins the behaviour of the handler helper that
// turns Echo's `*` wildcard into the literal filename before it flows into
// the WebDAV client. Echo returns its wildcard param already percent-
// encoded whenever Go's http parser sets r.URL.RawPath, which happens as
// soon as a character's default path encoding differs from what the client
// sent (e.g. `&` → %26 on the wire but Go leaves `&` unescaped). Pushing
// that encoded slice into url.URL.Path would double-encode `%` on the way
// out to Nextcloud; the helper decodes once at the boundary to prevent it.
func TestNextcloudPathParam(t *testing.T) {
	cases := []struct {
		name    string
		param   string
		want    string
		wantErr bool
	}{
		{"Ampersand", "Diagram%20%26%20table.ods", "Diagram & table.ods", false},
		{"LiteralApostrophe", "Mother's%20day.odt", "Mother's day.odt", false},
		{"Hash", "notes%23v2.md", "notes#v2.md", false},
		{"Parenthesis", "IMG%20%281%29.jpg", "IMG (1).jpg", false},
		{"Plain", "src.zip", "src.zip", false},
		{"Nested", "Photos/Frog.jpg", "Photos/Frog.jpg", false},
		{"MalformedPercent", "bad%zz", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
			c.SetParamNames("*")
			c.SetParamValues(tc.param)

			got, err := nextcloudPathParam(c)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
