package data

import (
	"net/url"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/gavv/httpexpect/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReferences(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	const Type = "io.cozy.events"
	const ID = "4521C325F6478E45"

	config.UseTestFile(t)
	testutils.NeedCouchdb(t)
	setup := testutils.NewSetup(t, t.Name())
	testInstance := setup.GetTestInstance()
	scope := "io.cozy.doctypes io.cozy.files io.cozy.events " +
		"io.cozy.anothertype io.cozy.nottype"

	_, token := setup.GetTestClient(scope)
	ts := setup.GetTestServer("/data", Routes)
	t.Cleanup(ts.Close)

	_ = couchdb.ResetDB(testInstance, Type)
	_ = couchdb.CreateNamedDoc(testInstance, &couchdb.JSONDoc{
		Type: Type,
		M: map[string]interface{}{
			"_id":  ID,
			"test": "testvalue",
		},
	})

	t.Run("ListNotSynchronizedOn", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Make doc
		doc := getDocForTest(Type, testInstance)

		// Make Files
		makeReferencedTestFile(t, testInstance, doc, "testtoref2.txt")
		makeReferencedTestFile(t, testInstance, doc, "testtoref3.txt")
		makeReferencedTestFile(t, testInstance, doc, "testtoref4.txt")
		makeReferencedTestFile(t, testInstance, doc, "testtoref5.txt")

		// Simple query
		obj := e.GET("/data/"+doc.DocType()+"/"+doc.ID()+"/relationships/references").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		obj.Path("$.meta.count").Equal(4)
		obj.Value("data").Array().Length().Equal(4)
		obj.Value("links").Object().NotContainsKey("next")

		// Use the page limit
		obj = e.GET("/data/"+doc.DocType()+"/"+doc.ID()+"/relationships/references").
			WithQuery("page[limit]", 3).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		obj.Path("$.meta.count").Equal(4)
		obj.Value("data").Array().Length().Equal(3)
		rawNext := obj.Value("links").Object().Value("next").String().NotEmpty().Raw()

		nextURL, err := url.Parse(rawNext)
		require.NoError(t, err)

		// Use the bookmark
		obj = e.GET(nextURL.Path).
			WithQueryString(nextURL.RawQuery).
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		obj.Value("data").Array().Length().Equal(1)
		obj.Value("links").Object().NotContainsKey("next")

		// Include the files
		obj = e.GET("/data/"+doc.DocType()+"/"+doc.ID()+"/relationships/references").
			WithQuery("include", "files").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		obj.Value("included").Array().Length().Equal(4)
		obj.Path("$.included[0].id").String().NotEmpty()
	})

	t.Run("AddReferencesHandler", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Make doc
		doc := getDocForTest(Type, testInstance)

		// Make File
		name := "testtoref.txt"
		dirID := consts.RootDirID
		mime, class := vfs.ExtractMimeAndClassFromFilename(name)
		filedoc, err := vfs.NewFileDoc(name, dirID, -1, nil, mime, class, time.Now(), false, false, false, nil)
		require.NoError(t, err)

		f, err := testInstance.VFS().CreateFile(filedoc, nil)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		// update it
		e.POST("/data/"+doc.DocType()+"/"+doc.ID()+"/relationships/references").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(`{
        "data": {
          "id": "` + filedoc.ID() + `",
          "type": "` + filedoc.DocType() + `"
        }
      }`)).
			Expect().Status(204)

		fdoc, err := testInstance.VFS().FileByID(filedoc.ID())
		assert.NoError(t, err)
		assert.Len(t, fdoc.ReferencedBy, 1)
	})

	t.Run("RemoveReferencesHandler", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Make doc
		doc := getDocForTest(Type, testInstance)

		// Make Files
		f6 := makeReferencedTestFile(t, testInstance, doc, "testtoref6.txt")
		f7 := makeReferencedTestFile(t, testInstance, doc, "testtoref7.txt")
		f8 := makeReferencedTestFile(t, testInstance, doc, "testtoref8.txt")
		f9 := makeReferencedTestFile(t, testInstance, doc, "testtoref9.txt")

		// update it
		e.DELETE("/data/"+doc.DocType()+"/"+doc.ID()+"/relationships/references").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(`{
        "data": [
          {"id": "` + f8 + `", "type": "` + consts.Files + `"},
          {"id": "` + f6 + `", "type": "` + consts.Files + `"}
        ]
      }`)).
			Expect().Status(204)

		fdoc6, err := testInstance.VFS().FileByID(f6)
		assert.NoError(t, err)
		assert.Len(t, fdoc6.ReferencedBy, 0)
		fdoc8, err := testInstance.VFS().FileByID(f8)
		assert.NoError(t, err)
		assert.Len(t, fdoc8.ReferencedBy, 0)

		fdoc7, err := testInstance.VFS().FileByID(f7)
		assert.NoError(t, err)
		assert.Len(t, fdoc7.ReferencedBy, 1)
		fdoc9, err := testInstance.VFS().FileByID(f9)
		assert.NoError(t, err)
		assert.Len(t, fdoc9.ReferencedBy, 1)
	})

	t.Run("ReferencesWithSlash", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Make File
		name := "test-ref-with-slash.txt"
		dirID := consts.RootDirID
		mime, class := vfs.ExtractMimeAndClassFromFilename(name)
		filedoc, err := vfs.NewFileDoc(name, dirID, -1, nil, mime, class, time.Now(), false, false, false, nil)
		require.NoError(t, err)

		f, err := testInstance.VFS().CreateFile(filedoc, nil)
		require.NoError(t, err)
		require.NoError(t, f.Close())

		// Add a reference to io.cozy.apps/foobar
		e.POST("/data/"+Type+"/io.cozy.apps%2ffoobar/relationships/references").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(`{
        "data": [
          {"id": "` + filedoc.ID() + `", "type": "` + filedoc.DocType() + `"}
        ]
      }`)).
			Expect().Status(204)

		fdoc, err := testInstance.VFS().FileByID(filedoc.ID())
		assert.NoError(t, err)
		assert.Len(t, fdoc.ReferencedBy, 1)
		assert.Equal(t, "io.cozy.apps/foobar", fdoc.ReferencedBy[0].ID)

		// Check that we can find the reference with /
		obj := e.GET("/data/"+Type+"/io.cozy.apps%2ffoobar/relationships/references").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		obj.Path("$.meta.count").Equal(1)
		obj.Value("data").Array().Length().Equal(1)
		obj.Path("$.data[0].id").Equal(fdoc.ID())

		// Try again, but this time encode / as %2F instead of %2f
		obj = e.GET("/data/"+Type+"/io.cozy.apps%2Ffoobar/relationships/references").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		obj.Path("$.meta.count").Equal(1)
		obj.Value("data").Array().Length().Equal(1)
		obj.Path("$.data[0].id").Equal(fdoc.ID())

		// Remove the reference with a /
		e.DELETE("/data/"+Type+"/io.cozy.apps%2Ffoobar/relationships/references").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(`{
        "data": [
          {"id": "` + fdoc.ID() + `", "type": "` + consts.Files + `"}
        ]
      }`)).
			Expect().Status(204)

		// Check that all the references have been removed
		fdoc2, err := testInstance.VFS().FileByID(fdoc.ID())
		assert.NoError(t, err)
		assert.Len(t, fdoc2.ReferencedBy, 0)
	})
}

func makeReferencedTestFile(t *testing.T, instance *instance.Instance, doc couchdb.Doc, name string) string {
	dirID := consts.RootDirID
	mime, class := vfs.ExtractMimeAndClassFromFilename(name)
	filedoc, err := vfs.NewFileDoc(name, dirID, -1, nil, mime, class, time.Now(), false, false, false, nil)
	if !assert.NoError(t, err) {
		return ""
	}

	filedoc.ReferencedBy = []couchdb.DocReference{
		{
			ID:   doc.ID(),
			Type: doc.DocType(),
		},
	}

	f, err := instance.VFS().CreateFile(filedoc, nil)
	if !assert.NoError(t, err) {
		return ""
	}
	if err = f.Close(); !assert.NoError(t, err) {
		return ""
	}
	return filedoc.ID()
}
