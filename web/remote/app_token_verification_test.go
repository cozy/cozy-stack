package remote

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/permission"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/tests/testutils"
	weberrors "github.com/cozy/cozy-stack/web/errors"
	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

// TestAppAudienceTokenPassesNextcloudPermissionCheck guards that an app
// audience token whose webapp permission doc holds io.cozy.files can reach
// GET /remote/nextcloud/:account/ without being rejected by the permission
// middleware.
func TestAppAudienceTokenPassesNextcloudPermissionCheck(t *testing.T) {
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

	rules := permission.Set{
		permission.Rule{Type: consts.Files, Verbs: permission.ALL},
		permission.Rule{Type: consts.NextcloudMigrations, Verbs: permission.ALL},
	}
	permReq := permission.Permission{
		Permissions: rules,
		Type:        permission.TypeWebapp,
		SourceID:    consts.Apps + "/migrator",
	}
	require.NoError(t, couchdb.CreateDoc(testInstance, &permReq))
	manifest := &couchdb.JSONDoc{
		Type: consts.Apps,
		M: map[string]interface{}{
			"_id":         consts.Apps + "/migrator",
			"slug":        "migrator",
			"permissions": rules,
		},
	}
	require.NoError(t, couchdb.CreateNamedDocWithDB(testInstance, manifest))
	token := testInstance.BuildAppToken("migrator", "")
	require.NotEmpty(t, token)

	mockWebDAV := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ocs/v2.php/cloud/user" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ocs":{"data":{"id":"migrator"}}}`))
			return
		}
		if r.Method == "PROPFIND" {
			body, _ := xml.Marshal(struct {
				XMLName xml.Name `xml:"d:multistatus"`
				Xmlns   string   `xml:"xmlns:d,attr"`
			}{Xmlns: "DAV:"})
			w.Header().Set("Content-Type", "application/xml; charset=utf-8")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write(body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(mockWebDAV.Close)

	accountDoc := &couchdb.JSONDoc{
		Type: consts.Accounts,
		M: map[string]interface{}{
			"account_type": "nextcloud",
			"name":         "Migration Source",
			"auth": map[string]interface{}{
				"login":    "migrator",
				"password": "secret",
				"url":      mockWebDAV.URL + "/",
			},
			"webdav_user_id": "migrator",
		},
	}
	account.Encrypt(*accountDoc)
	require.NoError(t, couchdb.CreateDoc(testInstance, accountDoc))
	accountID := accountDoc.ID()

	ts := setup.GetTestServer("/remote", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = weberrors.ErrorHandler
	t.Cleanup(ts.Close)

	e := testutils.CreateTestClient(t, ts.URL)

	e.GET("/remote/nextcloud/"+accountID+"/").
		WithHeader("Authorization", "Bearer "+token).
		WithHost(testInstance.Domain).
		Expect().Status(http.StatusOK).
		JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
		Object().
		Value("data").Array()
}
