package stream

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const input = `
{
  "total_rows": 6,
  "offset": 0,
  "rows": [
    {
      "id": "1bbde1890ad23cb023e0b2ee1d0cc1aa",
      "key": "1bbde1890ad23cb023e0b2ee1d0cc1aa",
      "value": {
        "rev": "1-14532ac999e880c2b731f0bd6ed5aef5"
      },
      "doc": {
        "_id": "1bbde1890ad23cb023e0b2ee1d0cc1aa",
        "_rev": "1-14532ac999e880c2b731f0bd6ed5aef5",
        "type": "file",
        "name": "happycloud.png",
        "dir_id": "ecbc071979a81554709a0394310c5291",
        "created_at": "2022-01-03T16:07:37.886Z",
        "updated_at": "2022-01-03T16:07:37.886Z",
        "size": "5167",
        "md5sum": "+ifa4cMPFbvDY8hERyfyGw==",
        "mime": "image/png",
        "class": "image",
        "executable": false,
        "trashed": false,
        "encrypted": false,
        "metadata": {
          "arrays": [[
            [{ "foo": "bar", "more": [] }],
            [{ "foo": "baz", "more": [] }]
          ]],
          "datetime": "2022-01-03T16:07:37.886Z",
          "extractor_version": 2,
          "height": 84,
          "width": 110
        },
        "cozyMetadata": {
          "doctypeVersion": "1",
          "metadataVersion": 1,
          "createdAt": "2022-10-24T17:32:27.187580797+02:00",
          "createdByApp": "drive",
          "updatedAt": "2022-10-24T17:32:27.187580797+02:00",
          "updatedByApps": [
            {
              "slug": "drive",
              "date": "2022-10-24T17:32:27.187580797+02:00",
              "instance": "http://joe.localhost:8080/"
            }
          ],
          "createdOn": "http://joe.localhost:8080/",
          "uploadedAt": "2022-10-24T17:32:27.187580797+02:00",
          "uploadedBy": {
            "slug": "drive"
          },
          "uploadedOn": "http://joe.localhost:8080/"
        }
      }
    },
    {
      "id": "_design/disk-usage",
      "key": "_design/disk-usage",
      "value": {
        "rev": "1-aa2ea41ec37738d50438e5b87fa5f544"
      },
      "doc": {
        "_id": "_design/disk-usage",
        "_rev": "1-aa2ea41ec37738d50438e5b87fa5f544",
        "language": "javascript",
        "views": {
          "disk-usage": {
            "map": "function(doc) { if (doc.type === 'file') { emit(doc.dir_id, +doc.size); } }",
            "reduce": "_sum"
          }
        }
      }
    },
    {
      "id": "_design/by-dir-id-updated-at",
      "key": "_design/by-dir-id-updated-at",
      "value": {
        "rev": "1-afade1d0263e3fb50ab52b61445aa4b9"
      },
      "doc": {
        "_id": "_design/by-dir-id-updated-at",
        "_rev": "1-afade1d0263e3fb50ab52b61445aa4b9",
        "language": "query",
        "views": {
          "5a9b035bcbceb460f09d8ae4f4ebf60ffa37125e": {
            "map": {
              "fields": {
                "dir_id": "asc",
                "updated_at": "asc"
              },
              "partial_filter_selector": {}
            },
            "reduce": "_count",
            "options": {
              "def": {
                "fields": [
                  "dir_id",
                  "updated_at"
                ]
              }
            }
          }
        }
      }
    },
    {
      "id": "ecbc071979a81554709a0394310c5291",
      "key": "ecbc071979a81554709a0394310c5291",
      "value": {
        "rev": "1-b894889f9135ca4ba915d6e7e96fba08"
      },
      "doc": {
        "_id": "ecbc071979a81554709a0394310c5291",
        "_rev": "1-b894889f9135ca4ba915d6e7e96fba08",
        "type": "directory",
        "name": "Photos",
        "dir_id": "io.cozy.files.root-dir",
        "created_at": "2022-09-19T13:45:54.565507012+02:00",
        "updated_at": "2022-09-19T13:45:54.565507012+02:00",
        "path": "/Photos",
        "cozyMetadata": {
          "doctypeVersion": "1",
          "metadataVersion": 1,
          "createdAt": "2022-09-19T13:45:54.565507834+02:00",
          "updatedAt": "2022-09-19T13:45:54.565507834+02:00",
          "createdOn": "http://joe.localhost:8080/"
        }
      }
    },
    {
      "id": "io.cozy.files.root-dir",
      "key": "io.cozy.files.root-dir",
      "value": {
        "rev": "1-1e0b279e7a36ccc415c1843f0355840f"
      },
      "doc": {
        "_id": "io.cozy.files.root-dir",
        "_rev": "1-1e0b279e7a36ccc415c1843f0355840f",
        "type": "directory",
        "created_at": "2022-09-19T13:45:54.54955097+02:00",
        "updated_at": "2022-09-19T13:45:54.54955097+02:00",
        "path": "/"
      }
    },
    {
      "id": "io.cozy.files.trash-dir",
      "key": "io.cozy.files.trash-dir",
      "value": {
        "rev": "1-11f72bef9b00c46cf8fe3c35c20bd86e"
      },
      "doc": {
        "_id": "io.cozy.files.trash-dir",
        "_rev": "1-11f72bef9b00c46cf8fe3c35c20bd86e",
        "type": "directory",
        "name": ".cozy_trash",
        "dir_id": "io.cozy.files.root-dir",
        "created_at": "2022-09-19T13:45:54.54955097+02:00",
        "updated_at": "2022-09-19T13:45:54.54955097+02:00",
        "path": "/.cozy_trash"
      }
    }
  ]
}`

func TestSkipDesingDocs(t *testing.T) {
	filter := NewAllDocsFilter(nil)
	filter.SkipDesignDocs()
	var w bytes.Buffer
	require.NoError(t, filter.Stream(strings.NewReader(input), &w))
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Bytes(), &data))
	assert.EqualValues(t, 0, data["offset"])
	assert.EqualValues(t, 4, data["total_rows"])
	rows, ok := data["rows"].([]interface{})
	require.True(t, ok)
	assert.Len(t, rows, 4)
	for i := range rows {
		row, ok := rows[i].(map[string]interface{})
		require.True(t, ok)
		id, ok := row["id"].(string)
		require.True(t, ok)
		assert.False(t, strings.HasPrefix(id, "_design/"))
		key, ok := row["key"].(string)
		require.True(t, ok)
		assert.Equal(t, id, key)
		value, ok := row["value"].(map[string]interface{})
		require.True(t, ok)
		assert.Contains(t, value["rev"], "1-")
	}
}

func TestFilterBasicFields(t *testing.T) {
	filter := NewAllDocsFilter([]string{"_id", "type", "path"})
	var w bytes.Buffer
	require.NoError(t, filter.Stream(strings.NewReader(input), &w))
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Bytes(), &data))
	assert.EqualValues(t, 0, data["offset"])
	assert.EqualValues(t, 6, data["total_rows"])
	rows, ok := data["rows"].([]interface{})
	require.True(t, ok)
	assert.Len(t, rows, 6)

	row, ok := rows[4].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "io.cozy.files.root-dir", row["id"])
	assert.NotContains(t, row, "key")
	assert.NotContains(t, row, "value")
	doc, ok := row["doc"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "io.cozy.files.root-dir", doc["_id"])
	assert.Equal(t, "directory", doc["type"])
	assert.Equal(t, "/", doc["path"])
	assert.NotContains(t, doc, "_rev")
	assert.NotContains(t, doc, "name")
	assert.NotContains(t, doc, "dir_id")
	assert.NotContains(t, doc, "created_at")
	assert.NotContains(t, doc, "updated_at")

	row, ok = rows[5].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "io.cozy.files.trash-dir", row["id"])
	assert.NotContains(t, row, "key")
	assert.NotContains(t, row, "value")
	doc, ok = row["doc"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "io.cozy.files.trash-dir", doc["_id"])
	assert.Equal(t, "directory", doc["type"])
	assert.Equal(t, "/.cozy_trash", doc["path"])
	assert.NotContains(t, doc, "_rev")
	assert.NotContains(t, doc, "name")
	assert.NotContains(t, doc, "dir_id")
	assert.NotContains(t, doc, "created_at")
	assert.NotContains(t, doc, "updated_at")
}

func TestFilterDottedFields(t *testing.T) {
	filter := NewAllDocsFilter([]string{"metadata.datetime", "cozyMetadata.uploadedBy"})
	var w bytes.Buffer
	require.NoError(t, filter.Stream(strings.NewReader(input), &w))
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Bytes(), &data))
	assert.EqualValues(t, 0, data["offset"])
	assert.EqualValues(t, 6, data["total_rows"])
	rows, ok := data["rows"].([]interface{})
	require.True(t, ok)
	assert.Len(t, rows, 6)

	row, ok := rows[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "1bbde1890ad23cb023e0b2ee1d0cc1aa", row["id"])
	doc, ok := row["doc"].(map[string]interface{})
	require.True(t, ok)
	assert.NotContains(t, doc, "_id")
	assert.NotContains(t, doc, "_rev")
	assert.NotContains(t, doc, "name")
	assert.NotContains(t, doc, "dir_id")
	assert.NotContains(t, doc, "created_at")
	assert.NotContains(t, doc, "updated_at")
	metadata, ok := doc["metadata"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "2022-01-03T16:07:37.886Z", metadata["datetime"])
	assert.NotContains(t, metadata, "extractor_version")
	assert.NotContains(t, metadata, "height")
	assert.NotContains(t, metadata, "width")
	cozyMetadata, ok := doc["cozyMetadata"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "2022-01-03T16:07:37.886Z", metadata["datetime"])
	assert.NotContains(t, cozyMetadata, "doctypeVersion")
	assert.NotContains(t, cozyMetadata, "uploadedAt")
	assert.NotContains(t, cozyMetadata, "uploadedOn")
	uploadedBy, ok := cozyMetadata["uploadedBy"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "drive", uploadedBy["slug"])
}
