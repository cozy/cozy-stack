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

func TestReferencedby(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	var fileID1, fileID2 string
	var fileData1, fileData2 *httpexpect.Object

	config.UseTestFile()
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

	t.Run("AddReferencedByOneRelation", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/files/").
			WithQuery("Name", "toreference").
			WithQuery("Type", "file").
			WithHeader("Content-Type", "text/plain").
			WithHeader("Content-MD5", "UmfjCVWct/albVkURcJJfg==").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte("foo,bar")).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		fileData1 = obj.Value("data").Object()
		fileID1 = fileData1.Value("id").String().NotEmpty().Raw()
		meta1 := fileData1.Value("meta").Object()
		rev1 := meta1.Value("rev").String().Raw()

		obj = e.POST("/files/"+fileID1+"/relationships/referenced_by").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "data": {
          "id": "fooalbumid",
          "type": "io.cozy.photos.albums"
        }
      }`)).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		rev2 := obj.Path("$.meta.rev").NotEqual(rev1).String().Raw()
		obj.Path("$.meta.count").Equal(1)
		data := obj.Value("data").Array()
		data.Length().Equal(1)
		elem := data.First().Object()
		elem.ValueEqual("id", "fooalbumid")
		elem.ValueEqual("type", "io.cozy.photos.albums")

		doc, err := testInstance.VFS().FileByID(fileID1)
		assert.NoError(t, err)
		assert.Len(t, doc.ReferencedBy, 1)
		assert.Equal(t, doc.Rev(), rev2)
	})

	t.Run("AddReferencedByMultipleRelation", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.POST("/files/").
			WithQuery("Name", "toreference2").
			WithQuery("Type", "file").
			WithHeader("Content-Type", "text/plain").
			WithHeader("Content-MD5", "UmfjCVWct/albVkURcJJfg==").
			WithHeader("Authorization", "Bearer "+token).
			WithBytes([]byte("foo,bar")).
			Expect().Status(201).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		fileData2 = obj.Value("data").Object()
		fileID2 = fileData2.Value("id").String().NotEmpty().Raw()
		meta1 := fileData2.Value("meta").Object()
		rev1 := meta1.Value("rev").String().Raw()

		obj = e.POST("/files/"+fileID2+"/relationships/referenced_by").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "data": [ 
        { "id": "fooalbumid1", "type": "io.cozy.photos.albums" },
        { "id": "fooalbumid2", "type": "io.cozy.photos.albums" },
        { "id": "fooalbumid3", "type": "io.cozy.photos.albums" }
        ]
      }`)).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		rev2 := obj.Path("$.meta.rev").NotEqual(rev1).String().Raw()
		data := obj.Value("data").Array()
		data.Length().Equal(3)

		elem := data.Element(0).Object()
		elem.ValueEqual("id", "fooalbumid1")
		elem.ValueEqual("type", "io.cozy.photos.albums")
		elem = data.Element(1).Object()
		elem.ValueEqual("id", "fooalbumid2")
		elem.ValueEqual("type", "io.cozy.photos.albums")
		elem = data.Element(2).Object()
		elem.ValueEqual("id", "fooalbumid3")
		elem.ValueEqual("type", "io.cozy.photos.albums")

		doc, err := testInstance.VFS().FileByID(fileID2)
		assert.NoError(t, err)
		assert.Len(t, doc.ReferencedBy, 3)
		assert.Equal(t, doc.Rev(), rev2)
	})

	t.Run("RemoveReferencedByOneRelation", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.DELETE("/files/"+fileID1+"/relationships/referenced_by").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "data": {
          "id": "fooalbumid",
          "type": "io.cozy.photos.albums"
        }
      }`)).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		obj.Path("$.meta.count").Equal(0)
		obj.Value("data").Array().Empty()

		doc, err := testInstance.VFS().FileByID(fileID1)
		assert.NoError(t, err)
		assert.Len(t, doc.ReferencedBy, 0)
	})

	t.Run("RemoveReferencedByMultipleRelation", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		obj := e.DELETE("/files/"+fileID2+"/relationships/referenced_by").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/json").
			WithBytes([]byte(`{
        "data": [ 
        { "id": "fooalbumid3", "type": "io.cozy.photos.albums" },
        { "id": "fooalbumid5", "type": "io.cozy.photos.albums" },
        { "id": "fooalbumid1", "type": "io.cozy.photos.albums" }
        ]
      }`)).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		obj.Path("$.meta.count").Equal(1)
		data := obj.Value("data").Array()
		data.Length().Equal(1)
		elem := data.First().Object()
		elem.ValueEqual("id", "fooalbumid2")
		elem.ValueEqual("type", "io.cozy.photos.albums")

		doc, err := testInstance.VFS().FileByID(fileID2)
		assert.NoError(t, err)
		assert.Len(t, doc.ReferencedBy, 1)
		assert.Equal(t, "fooalbumid2", doc.ReferencedBy[0].ID)
	})
}
