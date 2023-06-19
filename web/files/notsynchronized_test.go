package files

import (
	"net/url"
	"testing"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gavv/httpexpect/v2"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotsynchronized(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	var dirID, dirID2 string

	config.UseTestFile(t)
	require.NoError(t, loadLocale(), "Could not load default locale translations")

	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())

	config.GetConfig().Fs.URL = &url.URL{
		Scheme: "file",
		Host:   "localhost",
		Path:   t.TempDir(),
	}

	testInstance := setup.GetTestInstance()
	_, token := setup.GetTestClient(consts.Files + " " + consts.CertifiedCarbonCopy + " " + consts.CertifiedElectronicSafe)
	ts := setup.GetTestServer("/files", Routes, func(r *echo.Echo) *echo.Echo {
		secure := middlewares.Secure(&middlewares.SecureConfig{
			CSPDefaultSrc:     []middlewares.CSPSource{middlewares.CSPSrcSelf},
			CSPFrameAncestors: []middlewares.CSPSource{middlewares.CSPSrcNone},
		})
		r.Use(secure)
		return r
	})
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler
	t.Cleanup(ts.Close)

	t.Run("AddNotSynchronizedOnOneRelation", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/files/").
			WithQuery("Name", "to_sync_or_not_to_sync_1").
			WithQuery("Type", "directory").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		dirID = obj.Path("$.data.id").String().NotEmpty().Raw()
		dirData := obj.Value("data").Object()

		obj = e.POST("/files/"+dirID+"/relationships/not_synchronized_on").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "data": {
          "id": "fooclientid",
          "type": "io.cozy.oauth.clients"
        }
      }`)).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		obj.Path("$.meta.rev").NotEqual(dirData.Path("$.meta.rev").String().Raw())
		obj.Path("$.meta.count").Equal(1)
		data := obj.Value("data").Array()
		data.Length().Equal(1)

		item := data.First().Object()
		item.ValueEqual("id", "fooclientid")
		item.ValueEqual("type", "io.cozy.oauth.clients")

		doc, err := testInstance.VFS().DirByID(dirID)
		assert.NoError(t, err)
		assert.Len(t, doc.NotSynchronizedOn, 1)
		assert.Equal(t, doc.Rev(), obj.Path("$.meta.rev").String().Raw())
	})

	t.Run("AddNotSynchronizedOnMultipleRelation", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/files/").
			WithQuery("Name", "to_sync_or_not_to_sync_2").
			WithQuery("Type", "directory").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		dirID2 = obj.Path("$.data.id").String().NotEmpty().Raw()
		dirData := obj.Value("data").Object()

		obj = e.POST("/files/"+dirID2+"/relationships/not_synchronized_on").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "data": [
        { "id": "fooclientid1", "type": "io.cozy.oauth.clients" },
        { "id": "fooclientid2", "type": "io.cozy.oauth.clients" },
        { "id": "fooclientid3", "type": "io.cozy.oauth.clients" }
        ]
      }`)).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		obj.Path("$.meta.rev").NotEqual(dirData.Path("$.meta.rev").String().Raw())
		obj.Path("$.meta.count").Equal(3)
		data := obj.Value("data").Array()
		data.Length().Equal(3)

		item := data.Element(0).Object()
		item.ValueEqual("id", "fooclientid1")
		item.ValueEqual("type", "io.cozy.oauth.clients")

		item = data.Element(1).Object()
		item.ValueEqual("id", "fooclientid2")
		item.ValueEqual("type", "io.cozy.oauth.clients")

		item = data.Element(2).Object()
		item.ValueEqual("id", "fooclientid3")
		item.ValueEqual("type", "io.cozy.oauth.clients")

		doc, err := testInstance.VFS().DirByID(dirID2)
		assert.NoError(t, err)
		assert.Len(t, doc.NotSynchronizedOn, 3)
		assert.Equal(t, doc.Rev(), obj.Path("$.meta.rev").String().Raw())
	})

	t.Run("RemoveNotSynchronizedOnOneRelation", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.DELETE("/files/"+dirID+"/relationships/not_synchronized_on").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "data": {
          "id": "fooclientid",
          "type": "io.cozy.oauth.clients"
        }
      }`)).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		obj.Path("$.meta.count").Equal(0)
		obj.Value("data").Array().Length().Equal(0)

		doc, err := testInstance.VFS().DirByID(dirID)
		assert.NoError(t, err)
		assert.Len(t, doc.NotSynchronizedOn, 0)
	})

	t.Run("RemoveNotSynchronizedOnMultipleRelation", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.DELETE("/files/"+dirID2+"/relationships/not_synchronized_on").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "data": [
        { "id": "fooclientid3", "type": "io.cozy.oauth.clients" },
        { "id": "fooclientid5", "type": "io.cozy.oauth.clients" },
        { "id": "fooclientid1", "type": "io.cozy.oauth.clients" }
        ]
      }`)).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		obj.Path("$.meta.count").Equal(1)
		data := obj.Value("data").Array()
		data.Length().Equal(1)
		data.First().Object().ValueEqual("id", "fooclientid2")
		data.First().Object().ValueEqual("type", "io.cozy.oauth.clients")

		doc, err := testInstance.VFS().DirByID(dirID2)
		assert.NoError(t, err)
		assert.Len(t, doc.NotSynchronizedOn, 1)
		assert.Equal(t, "fooclientid2", doc.NotSynchronizedOn[0].ID)
	})
}
