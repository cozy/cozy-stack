package data

import (
	"net/http"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
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
