package files

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/labstack/echo"
	"github.com/stretchr/testify/assert"
)

func TestAddReferencedByOneRelation(t *testing.T) {
	body := "foo,bar"
	res1, data1 := upload(t, "/files/?Type=file&Name=toreference", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	fileID, _ := extractDirData(t, data1)

	path := "/files/" + fileID + "/relationships/referenced_by"
	content, err := json.Marshal(&jsonapi.Relationship{
		Data: jsonapi.ResourceIdentifier{
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

	req.Header.Add(echo.HeaderAuthorization, "Bearer "+testToken(testInstance))

	res, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, 200, res.StatusCode)

}

func TestAddReferencedByMultipleRelation(t *testing.T) {
	body := "foo,bar"
	res1, data1 := upload(t, "/files/?Type=file&Name=toreference2", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	fileID, _ := extractDirData(t, data1)

	path := "/files/" + fileID + "/relationships/referenced_by"
	content, err := json.Marshal(&jsonapi.Relationship{
		Data: []jsonapi.ResourceIdentifier{
			{
				ID:   "fooalbumid",
				Type: "io.cozy.photos.albums",
			},
		},
	})
	if !assert.NoError(t, err) {
		return
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+path, bytes.NewReader(content))
	if !assert.NoError(t, err) {
		return
	}

	req.Header.Add(echo.HeaderAuthorization, "Bearer "+testToken(testInstance))

	res, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, 200, res.StatusCode)

}
