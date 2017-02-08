package data

import (
	"net/http"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/stretchr/testify/assert"
)

func TestAddReferencesHandler(t *testing.T) {

	// Make doc
	doc := getDocForTest()
	url := ts.URL + "/data/" + doc.DocType() + "/" + doc.ID() + "/relationships/references"

	// Make File
	name := "testtoref.txt"
	dirID := consts.RootDirID
	filedoc, err := vfs.NewFileDoc(name, dirID, -1, nil, "", "", time.Now(), false, nil)
	if !assert.NoError(t, err) {
		return
	}

	f, err := vfs.CreateFile(testInstance, filedoc, nil)
	if !assert.NoError(t, err) {
		return
	}
	if err = f.Close(); !assert.NoError(t, err) {
		return
	}

	// update it
	var in = jsonReader(jsonapi.Relationship{
		Data: []jsonapi.ResourceIdentifier{
			jsonapi.ResourceIdentifier{
				ID:   filedoc.ID(),
				Type: filedoc.DocType(),
			},
		},
	})
	req, _ := http.NewRequest("POST", url, in)
	req.Header.Add("Host", Host)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	res, err := http.DefaultClient.Do(req)
	defer res.Body.Close()

	assert.NoError(t, err)
	assert.Equal(t, 204, res.StatusCode)
}

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
	req.Header.Add("Host", Host)

	var result struct {
		Data []jsonapi.ResourceIdentifier `json:"data"`
	}

	_, res, err := doRequest(req, &result)

	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	assert.Len(t, result.Data, 4)

}

func makeReferencedTestFile(t *testing.T, doc couchdb.Doc, name string) {
	dirID := consts.RootDirID
	filedoc, err := vfs.NewFileDoc(name, dirID, -1, nil, "", "", time.Now(), false, nil)
	if !assert.NoError(t, err) {
		return
	}

	filedoc.ReferencedBy = []jsonapi.ResourceIdentifier{
		jsonapi.ResourceIdentifier{
			ID:   doc.ID(),
			Type: doc.DocType(),
		},
	}

	f, err := vfs.CreateFile(testInstance, filedoc, nil)
	if !assert.NoError(t, err) {
		return
	}
	if err = f.Close(); !assert.NoError(t, err) {
		return
	}
}
