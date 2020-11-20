package data

import (
	"net/http"
	"testing"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/stretchr/testify/assert"
)

func TestListNotSynchronizingHandler(t *testing.T) {
	// Make doc
	doc := getDocForTest()
	url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID() + "/relationships/not_synchronizing"

	// Make directories
	makeNotSynchronzedOnTestDir(t, doc, "test_not_sync_on_1")
	makeNotSynchronzedOnTestDir(t, doc, "test_not_sync_on_2")
	makeNotSynchronzedOnTestDir(t, doc, "test_not_sync_on_3")
	makeNotSynchronzedOnTestDir(t, doc, "test_not_sync_on_4")

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Add("Authorization", "Bearer "+token)

	var result struct {
		Links jsonapi.LinksList
		Data  []couchdb.DocReference `json:"data"`
		Meta  jsonapi.Meta
	}
	_, res, err := doRequest(req, &result)

	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	assert.Len(t, result.Data, 4)
	assert.Equal(t, *result.Meta.Count, 4)
	assert.Empty(t, result.Links.Next)

	var result2 struct {
		Links jsonapi.LinksList
		Data  []couchdb.DocReference `json:"data"`
	}
	req2, _ := http.NewRequest("GET", url+"?page[limit]=3", nil)
	req2.Header.Add("Authorization", "Bearer "+token)
	_, res2, err := doRequest(req2, &result2)

	assert.NoError(t, err)
	assert.Equal(t, 200, res2.StatusCode)
	assert.Len(t, result2.Data, 3)
	assert.Equal(t, *result.Meta.Count, 4)
	assert.NotEmpty(t, result2.Links.Next)

	var result3 struct {
		Links jsonapi.LinksList
		Data  []couchdb.DocReference `json:"data"`
	}
	req3, _ := http.NewRequest("GET", ts.URL+result2.Links.Next, nil)
	req3.Header.Add("Authorization", "Bearer "+token)
	_, res3, err := doRequest(req3, &result3)

	assert.NoError(t, err)
	assert.Equal(t, 200, res3.StatusCode)
	assert.Len(t, result3.Data, 1)
	assert.Empty(t, result3.Links.Next)

	var result4 struct {
		Links    jsonapi.LinksList
		Data     []couchdb.DocReference `json:"data"`
		Included []interface{}          `json:"included"`
	}
	req4, _ := http.NewRequest("GET", url+"?include=files", nil)
	req4.Header.Add("Authorization", "Bearer "+token)
	_, res4, err := doRequest(req4, &result4)

	assert.NoError(t, err)
	assert.Equal(t, 200, res4.StatusCode)
	assert.Len(t, result4.Included, 4)
	assert.NotEmpty(t, result4.Included[0].(map[string]interface{})["id"])
}

func TestAddNotSynchronizing(t *testing.T) {
	// Make doc
	doc := getDocForTest()
	url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID() + "/relationships/not_synchronizing"

	// Make dir
	dir := makeNotSynchronzedOnTestDir(t, nil, "test_not_sync_on")

	// update it
	in := jsonReader(jsonapi.Relationship{
		Data: []couchdb.DocReference{
			{
				ID:   dir.ID(),
				Type: dir.DocType(),
			},
		},
	})
	req, _ := http.NewRequest("POST", url, in)
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, 204, res.StatusCode)

	dirdoc, err := testInstance.VFS().DirByID(dir.ID())
	assert.NoError(t, err)
	assert.Len(t, dirdoc.NotSynchronizedOn, 1)
}

func TestRemoveNotSynchronizing(t *testing.T) {
	// Make doc
	doc := getDocForTest()
	url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID() + "/relationships/not_synchronizing"

	// Make directories
	d6 := makeNotSynchronzedOnTestDir(t, doc, "test_not_sync_on_6").ID()
	d7 := makeNotSynchronzedOnTestDir(t, doc, "test_not_sync_on_7").ID()
	d8 := makeNotSynchronzedOnTestDir(t, doc, "test_not_sync_on_8").ID()
	d9 := makeNotSynchronzedOnTestDir(t, doc, "test_not_sync_on_9").ID()

	// update it
	in := jsonReader(jsonapi.Relationship{
		Data: []couchdb.DocReference{
			{ID: d8, Type: consts.Files},
			{ID: d6, Type: consts.Files},
		},
	})
	req, _ := http.NewRequest("DELETE", url, in)
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, 204, res.StatusCode)

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
}

func makeNotSynchronzedOnTestDir(t *testing.T, doc couchdb.Doc, name string) *vfs.DirDoc {
	fs := testInstance.VFS()
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
