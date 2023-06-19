package data

import (
	"net/url"
	"testing"

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

func TestNot(t *testing.T) {
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

	t.Run("ListNotSynchronizingHandler", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Make doc
		doc := getDocForTest(Type, testInstance)

		// Make directories
		makeNotSynchronzedOnTestDir(t, testInstance, doc, "test_not_sync_on_1")
		makeNotSynchronzedOnTestDir(t, testInstance, doc, "test_not_sync_on_2")
		makeNotSynchronzedOnTestDir(t, testInstance, doc, "test_not_sync_on_3")
		makeNotSynchronzedOnTestDir(t, testInstance, doc, "test_not_sync_on_4")

		// Simple query
		obj := e.GET("/data/"+doc.DocType()+"/"+doc.ID()+"/relationships/not_synchronizing").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		obj.Path("$.meta.count").Equal(4)
		obj.Value("links").Object().NotContainsKey("next")
		obj.Value("data").Array().Length().Equal(4)

		// Use the page limit
		obj = e.GET("/data/"+doc.DocType()+"/"+doc.ID()+"/relationships/not_synchronizing").
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
		obj = e.GET("/data/"+doc.DocType()+"/"+doc.ID()+"/relationships/not_synchronizing").
			WithQuery("include", "files").
			WithHeader("Authorization", "Bearer "+token).
			Expect().Status(200).
			JSON(httpexpect.ContentOpts{MediaType: "application/vnd.api+json"}).
			Object()

		obj.Value("included").Array().Length().Equal(4)
		obj.Path("$.included[0].id").String().NotEmpty()
	})

	t.Run("AddNotSynchronizing", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Make doc
		doc := getDocForTest(Type, testInstance)

		// Make dir
		dir := makeNotSynchronzedOnTestDir(t, testInstance, nil, "test_not_sync_on")

		// update it
		e.POST("/data/"+doc.DocType()+"/"+doc.ID()+"/relationships/not_synchronizing").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(`{
        "data": {
          "id": "` + dir.ID() + `",
          "type": "` + dir.DocType() + `"
        }
      }`)).
			Expect().Status(204)

		dirdoc, err := testInstance.VFS().DirByID(dir.ID())
		assert.NoError(t, err)
		assert.Len(t, dirdoc.NotSynchronizedOn, 1)
	})

	t.Run("RemoveNotSynchronizing", func(t *testing.T) {
		e := testutils.CreateTestClient(t, ts.URL)

		// Make doc
		doc := getDocForTest(Type, testInstance)

		// Make directories
		d6 := makeNotSynchronzedOnTestDir(t, testInstance, doc, "test_not_sync_on_6").ID()
		d7 := makeNotSynchronzedOnTestDir(t, testInstance, doc, "test_not_sync_on_7").ID()
		d8 := makeNotSynchronzedOnTestDir(t, testInstance, doc, "test_not_sync_on_8").ID()
		d9 := makeNotSynchronzedOnTestDir(t, testInstance, doc, "test_not_sync_on_9").ID()

		// update it
		e.DELETE("/data/"+doc.DocType()+"/"+doc.ID()+"/relationships/not_synchronizing").
			WithHeader("Authorization", "Bearer "+token).
			WithHeader("Content-Type", "application/vnd.api+json").
			WithBytes([]byte(`{
        "data": [
          {"id": "` + d8 + `", "type": "` + consts.Files + `"},
          {"id": "` + d6 + `", "type": "` + consts.Files + `"}
        ]
      }`)).
			Expect().Status(204)

		doc6, err := testInstance.VFS().DirByID(d6)
		assert.NoError(t, err)
		assert.Len(t, doc6.NotSynchronizedOn, 0)
		doc8, err := testInstance.VFS().DirByID(d8)
		assert.NoError(t, err)
		assert.Len(t, doc8.NotSynchronizedOn, 0)

		doc7, err := testInstance.VFS().DirByID(d7)
		assert.NoError(t, err)
		assert.Len(t, doc7.NotSynchronizedOn, 1)
		doc9, err := testInstance.VFS().DirByID(d9)
		assert.NoError(t, err)
		assert.Len(t, doc9.NotSynchronizedOn, 1)
	})
}

func makeNotSynchronzedOnTestDir(t *testing.T, instance *instance.Instance, doc couchdb.Doc, name string) *vfs.DirDoc {
	fs := instance.VFS()
	dirID := consts.RootDirID
	dir, err := vfs.NewDirDoc(fs, name, dirID, nil)
	if !assert.NoError(t, err) {
		return nil
	}

	if doc != nil {
		dir.NotSynchronizedOn = []couchdb.DocReference{
			{
				ID:   doc.ID(),
				Type: doc.DocType(),
			},
		}
	}

	err = fs.CreateDir(dir)

	if !assert.NoError(t, err) {
		return nil
	}

	return dir
}
