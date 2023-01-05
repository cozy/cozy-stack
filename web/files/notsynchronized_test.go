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

var dirID1, dirID2 string

func TestNotsynchronized(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	t.Run("AddNotSynchronizedOnOneRelation", func(t *testing.T) {
		res1, data1 := createDir(t, "/files/?Type=directory&Name=to_sync_or_not_to_sync_1")
		require.Equal(t, 201, res1.StatusCode)

		var dirData1 map[string]interface{}
		dirID1, dirData1 = extractDirData(t, data1)

		path := "/files/" + dirID1 + "/relationships/not_synchronized_on"
		content, err := json.Marshal(&jsonapi.Relationship{
			Data: couchdb.DocReference{
				ID:   "fooclientid",
				Type: "io.cozy.oauth.clients",
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

		assert.NotEqual(t, result.Meta.Rev, dirData1["_rev"])
		assert.Equal(t, result.Meta.Count, 1)
		assert.Equal(t, result.Data, []couchdb.DocReference{
			{
				ID:   "fooclientid",
				Type: "io.cozy.oauth.clients",
			},
		})

		doc, err := testInstance.VFS().DirByID(dirID1)
		assert.NoError(t, err)
		assert.Len(t, doc.NotSynchronizedOn, 1)
		assert.Equal(t, doc.Rev(), result.Meta.Rev)
	})

	t.Run("AddNotSynchronizedOnMultipleRelation", func(t *testing.T) {
		res1, data1 := createDir(t, "/files/?Type=directory&Name=to_sync_or_not_to_sync_2")
		require.Equal(t, 201, res1.StatusCode)

		var dirData2 map[string]interface{}
		dirID2, dirData2 = extractDirData(t, data1)

		path := "/files/" + dirID2 + "/relationships/not_synchronized_on"
		content, err := json.Marshal(&jsonapi.Relationship{
			Data: []couchdb.DocReference{
				{ID: "fooclientid1", Type: "io.cozy.oauth.clients"},
				{ID: "fooclientid2", Type: "io.cozy.oauth.clients"},
				{ID: "fooclientid3", Type: "io.cozy.oauth.clients"},
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

		assert.NotEqual(t, result.Meta.Rev, dirData2["_rev"])
		assert.Equal(t, result.Meta.Count, 3)
		assert.Equal(t, result.Data, []couchdb.DocReference{
			{ID: "fooclientid1", Type: "io.cozy.oauth.clients"},
			{ID: "fooclientid2", Type: "io.cozy.oauth.clients"},
			{ID: "fooclientid3", Type: "io.cozy.oauth.clients"},
		})

		doc, err := testInstance.VFS().DirByID(dirID2)
		assert.NoError(t, err)
		assert.Len(t, doc.NotSynchronizedOn, 3)
		assert.Equal(t, doc.Rev(), result.Meta.Rev)
	})

	t.Run("RemoveNotSynchronizedOnOneRelation", func(t *testing.T) {
		path := "/files/" + dirID1 + "/relationships/not_synchronized_on"
		content, err := json.Marshal(&jsonapi.Relationship{
			Data: couchdb.DocReference{
				ID:   "fooclientid",
				Type: "io.cozy.oauth.clients",
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

		doc, err := testInstance.VFS().DirByID(dirID1)
		assert.NoError(t, err)
		assert.Len(t, doc.NotSynchronizedOn, 0)
	})

	t.Run("RemoveNotSynchronizedOnMultipleRelation", func(t *testing.T) {
		path := "/files/" + dirID2 + "/relationships/not_synchronized_on"
		content, err := json.Marshal(&jsonapi.Relationship{
			Data: []couchdb.DocReference{
				{ID: "fooclientid3", Type: "io.cozy.oauth.clients"},
				{ID: "fooclientid5", Type: "io.cozy.oauth.clients"},
				{ID: "fooclientid1", Type: "io.cozy.oauth.clients"},
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
			{ID: "fooclientid2", Type: "io.cozy.oauth.clients"},
		})

		doc, err := testInstance.VFS().DirByID(dirID2)
		assert.NoError(t, err)
		assert.Len(t, doc.NotSynchronizedOn, 1)
		assert.Equal(t, "fooclientid2", doc.NotSynchronizedOn[0].ID)
	})
}
