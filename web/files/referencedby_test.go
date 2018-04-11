package files

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/echo"
	"github.com/stretchr/testify/assert"
)

var fileID1, fileID2 string

func TestAddReferencedByOneRelation(t *testing.T) {
	body := "foo,bar"
	res1, data1 := upload(t, "/files/?Type=file&Name=toreference", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	fileID1, _ = extractDirData(t, data1)

	path := "/files/" + fileID1 + "/relationships/referenced_by"
	content, err := json.Marshal(&jsonapi.Relationship{
		Data: couchdb.DocReference{
			ID:   "fooalbumid",
			Type: "io.cozy.photos.albums",
		},
	})
	if !assert.NoError(t, err) {
		return
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+path, bytes.NewReader(content))
	if !assert.NoError(t, err) {
		return
	}
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)

	res, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, 204, res.StatusCode)

	doc, err := testInstance.VFS().FileByID(fileID1)
	assert.NoError(t, err)
	assert.Len(t, doc.ReferencedBy, 1)
}

func TestAddReferencedByMultipleRelation(t *testing.T) {
	body := "foo,bar"
	res1, data1 := upload(t, "/files/?Type=file&Name=toreference2", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	fileID2, _ = extractDirData(t, data1)

	path := "/files/" + fileID2 + "/relationships/referenced_by"
	content, err := json.Marshal(&jsonapi.Relationship{
		Data: []couchdb.DocReference{
			{ID: "fooalbumid1", Type: "io.cozy.photos.albums"},
			{ID: "fooalbumid2", Type: "io.cozy.photos.albums"},
			{ID: "fooalbumid3", Type: "io.cozy.photos.albums"},
		},
	})
	if !assert.NoError(t, err) {
		return
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+path, bytes.NewReader(content))
	if !assert.NoError(t, err) {
		return
	}
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)

	res, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, 204, res.StatusCode)

	doc, err := testInstance.VFS().FileByID(fileID2)
	assert.NoError(t, err)
	assert.Len(t, doc.ReferencedBy, 3)
}

func TestRemoveReferencedByOneRelation(t *testing.T) {
	path := "/files/" + fileID1 + "/relationships/referenced_by"
	content, err := json.Marshal(&jsonapi.Relationship{
		Data: couchdb.DocReference{
			ID:   "fooalbumid",
			Type: "io.cozy.photos.albums",
		},
	})
	assert.NoError(t, err)

	req, err := http.NewRequest(http.MethodDelete, ts.URL+path, bytes.NewReader(content))
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 204, res.StatusCode)

	doc, err := testInstance.VFS().FileByID(fileID1)
	assert.NoError(t, err)
	assert.Len(t, doc.ReferencedBy, 0)
}

func TestRemoveReferencedByMultipleRelation(t *testing.T) {
	path := "/files/" + fileID2 + "/relationships/referenced_by"
	content, err := json.Marshal(&jsonapi.Relationship{
		Data: []couchdb.DocReference{
			{ID: "fooalbumid3", Type: "io.cozy.photos.albums"},
			{ID: "fooalbumid5", Type: "io.cozy.photos.albums"},
			{ID: "fooalbumid1", Type: "io.cozy.photos.albums"},
		},
	})
	assert.NoError(t, err)

	req, err := http.NewRequest(http.MethodDelete, ts.URL+path, bytes.NewReader(content))
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 204, res.StatusCode)

	doc, err := testInstance.VFS().FileByID(fileID2)
	assert.NoError(t, err)
	assert.Len(t, doc.ReferencedBy, 1)
	assert.Equal(t, "fooalbumid2", doc.ReferencedBy[0].ID)
}
