package intents

import (
	"testing"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestIntents(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	var err error
	var intentID string

	config.UseTestFile()
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	ins := setup.GetTestInstance(&lifecycle.Options{
		Domain: "cozy.example.net",
	})
	_, _ = setup.GetTestClient(consts.Settings)

	webapp := &couchdb.JSONDoc{
		Type: consts.Apps,
		M: map[string]interface{}{
			"_id":  consts.Apps + "/app",
			"slug": "app",
		},
	}
	require.NoError(t, couchdb.CreateNamedDoc(ins, webapp))

	appPerms, err := permission.CreateWebappSet(ins, "app", permission.Set{}, "1.0.0")
	if err != nil {
		require.NoError(t, err)
	}
	appToken := ins.BuildAppToken("app", "")
	files := &couchdb.JSONDoc{
		Type: consts.Apps,
		M: map[string]interface{}{
			"_id":  consts.Apps + "/files",
			"slug": "files",
			"intents": []app.Intent{
				{
					Action: "PICK",
					Types:  []string{"io.cozy.files", "image/gif"},
					Href:   "/pick",
				},
			},
		},
	}

	require.NoError(t, couchdb.CreateNamedDoc(ins, files))
	if _, err := permission.CreateWebappSet(ins, "files", permission.Set{}, "1.0.0"); err != nil {
		require.NoError(t, err)
	}
	filesToken := ins.BuildAppToken("files", "")

	ts := setup.GetTestServer("/intents", Routes)
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(ts.Close)

	t.Run("CreateIntent", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/intents").
			WithHeader("Authorization", "Bearer "+appToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithHeader("Accept", "application/vnd.api+json").
			WithBytes([]byte(`{
        "data": {
          "type": "io.cozy.settings",
          "attributes": {
            "action": "PICK",
            "type": "io.cozy.files",
            "permissions": ["GET"]
          }
        }
      }`)).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		intentID = checkIntentResult(obj, appPerms, true)
	})

	t.Run("GetIntent", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.GET("/intents/"+intentID).
			WithHeader("Authorization", "Bearer "+filesToken).
			WithHeader("Accept", "application/vnd.api+json").
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		checkIntentResult(obj, appPerms, true)
	})

	t.Run("GetIntentNotFromTheService", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		e.GET("/intents/"+intentID).
			WithHeader("Authorization", "Bearer "+appToken).
			WithHeader("Accept", "application/vnd.api+json").
			Expect().Status(403)
	})

	t.Run("CreateIntentOAuth", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/intents").
			WithHeader("Authorization", "Bearer "+appToken).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithHeader("Accept", "application/vnd.api+json").
			WithBytes([]byte(`{
        "data": {
          "type": "io.cozy.settings",
          "attributes": {
            "action": "PICK",
            "type": "io.cozy.files",
            "permissions": ["GET"]
          }
        }
      }`)).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		checkIntentResult(obj, appPerms, false)
	})
}

func checkIntentResult(obj *httpexpect.Object, appPerms *permission.Permission, fromWeb bool) string {
	data := obj.Value("data").Object()
	data.ValueEqual("type", "io.cozy.intents")
	intentID := data.Value("id").String().NotEmpty().Raw()

	attrs := data.Value("attributes").Object()
	attrs.ValueEqual("action", "PICK")
	attrs.ValueEqual("type", "io.cozy.files")

	perms := attrs.Value("permissions").Array()
	perms.Length().Equal(1)
	perms.First().String().Equal("GET")

	if !fromWeb {
		return intentID
	}

	attrs.ValueEqual("client", "https://app.cozy.example.net")

	links := data.Value("links").Object()
	links.ValueEqual("self", "/intents/"+intentID)
	links.ValueEqual("permissions", "/permissions/"+appPerms.ID())

	return intentID
}
