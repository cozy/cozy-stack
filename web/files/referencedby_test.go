package files

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var fileID1, fileID2 string
var fileData1, fileData2 map[string]interface{}

func TestReferencedby(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	t.Run("AddReferencedByOneRelation", func(t *testing.T) {
		body := "foo,bar"
		res1, data1 := upload(t, "/files/?Type=file&Name=toreference", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
		require.Equal(t, 201, res1.StatusCode)

		fileID1, fileData1 = extractDirData(t, data1)

		path := "/files/" + fileID1 + "/relationships/referenced_by"
		content, err := json.Marshal(&jsonapi.Relationship{
			Data: couchdb.DocReference{
				ID:   "fooalbumid",
				Type: "io.cozy.photos.albums",
			},
		})
		require.NoError(t, err)

		var result struct {
			Data []couchdb.DocReference `json:"data"`
			Meta struct {
				Rev   string `json:"rev"`
				Count int    `json:"count"`
			} `json:"meta"`
		}
		req, err := http.NewRequest(http.MethodPost, ts.URL+path, bytes.NewReader(content))
		require.NoError(t, err)

		req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)

		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)

		assert.Equal(t, 200, res.StatusCode)
		err = json.NewDecoder(res.Body).Decode(&result)
		require.NoError(t, err)

		assert.NotEqual(t, result.Meta.Rev, fileData1["_rev"])
		assert.Equal(t, result.Meta.Count, 1)
		assert.Equal(t, result.Data, []couchdb.DocReference{
			{
				ID:   "fooalbumid",
				Type: "io.cozy.photos.albums",
			},
		})

		doc, err := testInstance.VFS().FileByID(fileID1)
		assert.NoError(t, err)
		assert.Len(t, doc.ReferencedBy, 1)
		assert.Equal(t, doc.Rev(), result.Meta.Rev)
	})

	t.Run("AddReferencedByMultipleRelation", func(t *testing.T) {
		body := "foo,bar"
		res1, data1 := upload(t, "/files/?Type=file&Name=toreference2", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
		require.Equal(t, 201, res1.StatusCode)

		fileID2, fileData2 = extractDirData(t, data1)

		path := "/files/" + fileID2 + "/relationships/referenced_by"
		content, err := json.Marshal(&jsonapi.Relationship{
			Data: []couchdb.DocReference{
				{ID: "fooalbumid1", Type: "io.cozy.photos.albums"},
				{ID: "fooalbumid2", Type: "io.cozy.photos.albums"},
				{ID: "fooalbumid3", Type: "io.cozy.photos.albums"},
			},
		})
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodPost, ts.URL+path, bytes.NewReader(content))
		require.NoError(t, err)

		req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)

		var result struct {
			Data []couchdb.DocReference `json:"data"`
			Meta struct {
				Rev   string `json:"rev"`
				Count int    `json:"count"`
			} `json:"meta"`
		}
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)

		assert.Equal(t, 200, res.StatusCode)
		err = json.NewDecoder(res.Body).Decode(&result)
		require.NoError(t, err)

		assert.NotEqual(t, result.Meta.Rev, fileData2["_rev"])
		assert.Equal(t, result.Meta.Count, 3)
		assert.Equal(t, result.Data, []couchdb.DocReference{
			{ID: "fooalbumid1", Type: "io.cozy.photos.albums"},
			{ID: "fooalbumid2", Type: "io.cozy.photos.albums"},
			{ID: "fooalbumid3", Type: "io.cozy.photos.albums"},
		})

		doc, err := testInstance.VFS().FileByID(fileID2)
		assert.NoError(t, err)
		assert.Len(t, doc.ReferencedBy, 3)
		assert.Equal(t, doc.Rev(), result.Meta.Rev)
	})

	t.Run("RemoveReferencedByOneRelation", func(t *testing.T) {
		path := "/files/" + fileID1 + "/relationships/referenced_by"
		content, err := json.Marshal(&jsonapi.Relationship{
			Data: couchdb.DocReference{
				ID:   "fooalbumid",
				Type: "io.cozy.photos.albums",
			},
		})
		assert.NoError(t, err)

		var result struct {
			Data []couchdb.DocReference `json:"data"`
			Meta struct {
				Rev   string `json:"rev"`
				Count int    `json:"count"`
			} `json:"meta"`
		}
		req, err := http.NewRequest(http.MethodDelete, ts.URL+path, bytes.NewReader(content))
		assert.NoError(t, err)
		req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, 200, res.StatusCode)
		err = json.NewDecoder(res.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, result.Meta.Count, 0)
		assert.Equal(t, result.Data, []couchdb.DocReference{})

		doc, err := testInstance.VFS().FileByID(fileID1)
		assert.NoError(t, err)
		assert.Len(t, doc.ReferencedBy, 0)
	})

	t.Run("RemoveReferencedByMultipleRelation", func(t *testing.T) {
		path := "/files/" + fileID2 + "/relationships/referenced_by"
		content, err := json.Marshal(&jsonapi.Relationship{
			Data: []couchdb.DocReference{
				{ID: "fooalbumid3", Type: "io.cozy.photos.albums"},
				{ID: "fooalbumid5", Type: "io.cozy.photos.albums"},
				{ID: "fooalbumid1", Type: "io.cozy.photos.albums"},
			},
		})
		assert.NoError(t, err)

		var result struct {
			Data []couchdb.DocReference `json:"data"`
			Meta struct {
				Rev   string `json:"rev"`
				Count int    `json:"count"`
			} `json:"meta"`
		}
		req, err := http.NewRequest(http.MethodDelete, ts.URL+path, bytes.NewReader(content))
		assert.NoError(t, err)
		req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, 200, res.StatusCode)
		err = json.NewDecoder(res.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, result.Meta.Count, 1)
		assert.Equal(t, result.Data, []couchdb.DocReference{
			{ID: "fooalbumid2", Type: "io.cozy.photos.albums"},
		})

		doc, err := testInstance.VFS().FileByID(fileID2)
		assert.NoError(t, err)
		assert.Len(t, doc.ReferencedBy, 1)
		assert.Equal(t, "fooalbumid2", doc.ReferencedBy[0].ID)
	})

}
