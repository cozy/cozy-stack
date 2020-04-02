package data

import (
	"net/http"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/stretchr/testify/assert"
)

func TestListReferencesHandler(t *testing.T) {
	// Make doc
	doc := getDocForTest()
	url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID() + "/relationships/references"

	// Make Files
	makeReferencedTestFile(t, doc, "testtoref2.txt")
	makeReferencedTestFile(t, doc, "testtoref3.txt")
	makeReferencedTestFile(t, doc, "testtoref4.txt")
	makeReferencedTestFile(t, doc, "testtoref5.txt")

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

func TestAddReferencesHandler(t *testing.T) {
	// Make doc
	doc := getDocForTest()
	url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID() + "/relationships/references"

	// Make File
	name := "testtoref.txt"
	dirID := consts.RootDirID
	filedoc, err := vfs.NewFileDoc(name, dirID, -1, nil, "", "", time.Now(), false, false, nil)
	if !assert.NoError(t, err) {
		return
	}

	f, err := testInstance.VFS().CreateFile(filedoc, nil)
	if !assert.NoError(t, err) {
		return
	}
	if err = f.Close(); !assert.NoError(t, err) {
		return
	}

	// update it
	var in = jsonReader(jsonapi.Relationship{
		Data: []couchdb.DocReference{
			{
				ID:   filedoc.ID(),
				Type: filedoc.DocType(),
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

	fdoc, err := testInstance.VFS().FileByID(filedoc.ID())
	assert.NoError(t, err)
	assert.Len(t, fdoc.ReferencedBy, 1)
}

func TestRemoveReferencesHandler(t *testing.T) {
	// Make doc
	doc := getDocForTest()
	url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID() + "/relationships/references"

	// Make Files
	f6 := makeReferencedTestFile(t, doc, "testtoref6.txt")
	f7 := makeReferencedTestFile(t, doc, "testtoref7.txt")
	f8 := makeReferencedTestFile(t, doc, "testtoref8.txt")
	f9 := makeReferencedTestFile(t, doc, "testtoref9.txt")

	// update it
	var in = jsonReader(jsonapi.Relationship{
		Data: []couchdb.DocReference{
			{ID: f8, Type: consts.Files},
			{ID: f6, Type: consts.Files},
		},
	})
	req, _ := http.NewRequest("DELETE", url, in)
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, 204, res.StatusCode)

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
}

func TestReferencesWithSlash(t *testing.T) {
	// Make File
	name := "test-ref-with-slash.txt"
	dirID := consts.RootDirID
	filedoc, err := vfs.NewFileDoc(name, dirID, -1, nil, "", "", time.Now(), false, false, nil)
	if !assert.NoError(t, err) {
		return
	}
	f, err := testInstance.VFS().CreateFile(filedoc, nil)
	if !assert.NoError(t, err) {
		return
	}
	if err = f.Close(); !assert.NoError(t, err) {
		return
	}

	// Add a reference to io.cozy.apps/foobar
	url := ts.URL + "/data/" + Type + "/io.cozy.apps%2ffoobar/relationships/references"
	var in = jsonReader(jsonapi.Relationship{
		Data: []couchdb.DocReference{
			{
				ID:   filedoc.ID(),
				Type: filedoc.DocType(),
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

	fdoc, err := testInstance.VFS().FileByID(filedoc.ID())
	assert.NoError(t, err)
	assert.Len(t, fdoc.ReferencedBy, 1)
	assert.Equal(t, "io.cozy.apps/foobar", fdoc.ReferencedBy[0].ID)

	// Check that we can find the reference with /
	var result struct {
		Data []couchdb.DocReference `json:"data"`
		Meta jsonapi.Meta
	}
	req, _ = http.NewRequest("GET", url, nil)
	req.Header.Add("Authorization", "Bearer "+token)
	_, res, err = doRequest(req, &result)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	assert.Len(t, result.Data, 1)
	assert.Equal(t, *result.Meta.Count, 1)
	assert.Equal(t, fdoc.ID(), result.Data[0].ID)

	// Try again, but this time encode / as %2F instead of %2f
	url = ts.URL + "/data/" + Type + "/io.cozy.apps%2Ffoobar/relationships/references"
	result.Data = result.Data[:0]
	result.Meta = jsonapi.Meta{}
	req, _ = http.NewRequest("GET", url, nil)
	req.Header.Add("Authorization", "Bearer "+token)
	_, res, err = doRequest(req, &result)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	assert.Len(t, result.Data, 1)
	assert.Equal(t, *result.Meta.Count, 1)
	assert.Equal(t, fdoc.ID(), result.Data[0].ID)

	// Add dummy references on io.cozy.apps%2ffoobaz and io.cozy.apps%2Ffooqux
	foobazRef := couchdb.DocReference{
		ID:   "io.cozy.apps%2ffoobaz",
		Type: Type,
	}
	fooquxRef := couchdb.DocReference{
		ID:   "io.cozy.apps%2Ffooqux",
		Type: Type,
	}
	fdoc.ReferencedBy = append(fdoc.ReferencedBy, foobazRef, fooquxRef)
	err = couchdb.UpdateDoc(testInstance, fdoc)
	assert.NoError(t, err)

	// Check that we can find the reference with %2f
	url2 := ts.URL + "/data/" + Type + "/io.cozy.apps%2ffoobaz/relationships/references"
	result.Data = result.Data[:0]
	result.Meta = jsonapi.Meta{}
	req, _ = http.NewRequest("GET", url2, nil)
	req.Header.Add("Authorization", "Bearer "+token)
	_, res, err = doRequest(req, &result)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	assert.Len(t, result.Data, 1)
	assert.Equal(t, *result.Meta.Count, 1)
	assert.Equal(t, fdoc.ID(), result.Data[0].ID)

	// Check that we can find the reference with %2F
	url3 := ts.URL + "/data/" + Type + "/io.cozy.apps%2Ffooqux/relationships/references"
	result.Data = result.Data[:0]
	result.Meta = jsonapi.Meta{}
	req, _ = http.NewRequest("GET", url3, nil)
	req.Header.Add("Authorization", "Bearer "+token)
	_, res, err = doRequest(req, &result)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	assert.Len(t, result.Data, 1)
	assert.Equal(t, *result.Meta.Count, 1)
	assert.Equal(t, fdoc.ID(), result.Data[0].ID)

	// Remove the reference with a /
	in = jsonReader(jsonapi.Relationship{
		Data: []couchdb.DocReference{
			{ID: fdoc.ID(), Type: consts.Files},
		},
	})
	req, _ = http.NewRequest("DELETE", url, in)
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/vnd.api+json")
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, 204, res.StatusCode)

	// Remove the reference with a %2f
	in = jsonReader(jsonapi.Relationship{
		Data: []couchdb.DocReference{
			{ID: fdoc.ID(), Type: consts.Files},
		},
	})
	req, _ = http.NewRequest("DELETE", url2, in)
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/vnd.api+json")
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, 204, res.StatusCode)

	// Remove the reference with a %2F
	in = jsonReader(jsonapi.Relationship{
		Data: []couchdb.DocReference{
			{ID: fdoc.ID(), Type: consts.Files},
		},
	})
	req, _ = http.NewRequest("DELETE", url3, in)
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/vnd.api+json")
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, 204, res.StatusCode)

	// Check that all the references have been removed
	fdoc2, err := testInstance.VFS().FileByID(fdoc.ID())
	assert.NoError(t, err)
	assert.Len(t, fdoc2.ReferencedBy, 0)
}

func makeReferencedTestFile(t *testing.T, doc couchdb.Doc, name string) string {
	dirID := consts.RootDirID
	filedoc, err := vfs.NewFileDoc(name, dirID, -1, nil, "", "", time.Now(), false, false, nil)
	if !assert.NoError(t, err) {
		return ""
	}

	filedoc.ReferencedBy = []couchdb.DocReference{
		{
			ID:   doc.ID(),
			Type: doc.DocType(),
		},
	}

	f, err := testInstance.VFS().CreateFile(filedoc, nil)
	if !assert.NoError(t, err) {
		return ""
	}
	if err = f.Close(); !assert.NoError(t, err) {
		return ""
	}
	return filedoc.ID()
}
