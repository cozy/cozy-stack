package files

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/cozy/cozy-stack/web/errors"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
	"github.com/stretchr/testify/assert"

	_ "github.com/cozy/cozy-stack/worker/thumbnail"
)

var ts *httptest.Server
var testInstance *instance.Instance
var token string
var clientID string
var imgID string
var fileID string
var publicToken string

func readFile(fs vfs.VFS, name string) ([]byte, error) {
	doc, err := fs.FileByPath(name)
	if err != nil {
		return nil, err
	}
	f, err := fs.OpenFile(doc)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ioutil.ReadAll(f)
}

func extractJSONRes(res *http.Response, mp *map[string]interface{}) error {
	if res.StatusCode >= 300 {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(mp)
}

func createDir(t *testing.T, path string) (res *http.Response, v map[string]interface{}) {
	req, err := http.NewRequest("POST", ts.URL+path, strings.NewReader(""))
	if !assert.NoError(t, err) {
		return
	}
	req.Header.Add("Content-Type", "text/plain")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	res, err = http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	defer res.Body.Close()

	err = extractJSONRes(res, &v)
	assert.NoError(t, err)

	return
}

func doUploadOrMod(t *testing.T, req *http.Request, contentType, hash string) (res *http.Response, v map[string]interface{}) {
	var err error

	if contentType != "" {
		req.Header.Add("Content-Type", contentType)
	}

	if hash != "" {
		req.Header.Add("Content-MD5", hash)
	}

	res, err = http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}

	defer res.Body.Close()

	err = extractJSONRes(res, &v)
	assert.NoError(t, err)

	return
}

func upload(t *testing.T, path, contentType, body, hash string) (res *http.Response, v map[string]interface{}) {
	buf := strings.NewReader(body)
	req, err := http.NewRequest("POST", ts.URL+path, buf)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	if !assert.NoError(t, err) {
		return
	}
	return doUploadOrMod(t, req, contentType, hash)
}

func uploadMod(t *testing.T, path, contentType, body, hash string) (res *http.Response, v map[string]interface{}) {
	buf := strings.NewReader(body)
	req, err := http.NewRequest("PUT", ts.URL+path, buf)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	if !assert.NoError(t, err) {
		return
	}
	return doUploadOrMod(t, req, contentType, hash)
}

func trash(t *testing.T, path string) (res *http.Response, v map[string]interface{}) {
	req, err := http.NewRequest(http.MethodDelete, ts.URL+path, nil)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	if !assert.NoError(t, err) {
		return
	}

	res, err = http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}

	err = extractJSONRes(res, &v)
	assert.NoError(t, err)

	return
}

func restore(t *testing.T, path string) (res *http.Response, v map[string]interface{}) {
	req, err := http.NewRequest(http.MethodPost, ts.URL+path, nil)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	if !assert.NoError(t, err) {
		return
	}

	res, err = http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}

	err = extractJSONRes(res, &v)
	assert.NoError(t, err)

	return
}

func extractDirData(t *testing.T, data map[string]interface{}) (string, map[string]interface{}) {
	var ok bool

	data, ok = data["data"].(map[string]interface{})
	if !assert.True(t, ok) {
		return "", nil
	}

	id, ok := data["id"].(string)
	if !assert.True(t, ok) {
		return "", nil
	}

	return id, data
}

type jsonData struct {
	Type  string                 `json:"type"`
	ID    string                 `json:"id"`
	Attrs map[string]interface{} `json:"attributes,omitempty"`
	Rels  map[string]interface{} `json:"relationships,omitempty"`
}

func patchFile(t *testing.T, path, docType, id string, attrs map[string]interface{}, parent *jsonData) (res *http.Response, v map[string]interface{}) {
	bodyreq := &jsonData{
		Type:  docType,
		ID:    id,
		Attrs: attrs,
	}

	if parent != nil {
		bodyreq.Rels = map[string]interface{}{
			"parent": map[string]interface{}{
				"data": parent,
			},
		}
	}

	b, err := json.Marshal(map[string]*jsonData{"data": bodyreq})
	if !assert.NoError(t, err) {
		return
	}

	req, err := http.NewRequest("PATCH", ts.URL+path, bytes.NewReader(b))
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	if !assert.NoError(t, err) {
		return
	}

	res, err = http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}

	defer res.Body.Close()

	err = extractJSONRes(res, &v)
	assert.NoError(t, err)

	return
}

func download(t *testing.T, path, byteRange string) (res *http.Response, body []byte) {
	req, err := http.NewRequest("GET", ts.URL+path, nil)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	if !assert.NoError(t, err) {
		return
	}

	if byteRange != "" {
		req.Header.Add("Range", byteRange)
	}

	res, err = http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}

	body, err = ioutil.ReadAll(res.Body)
	if !assert.NoError(t, err) {
		return
	}

	return
}

func TestCreateDirWithNoType(t *testing.T) {
	res, _ := createDir(t, "/files/")
	assert.Equal(t, 422, res.StatusCode)
}

func TestCreateDirWithNoName(t *testing.T) {
	res, _ := createDir(t, "/files/?Type=directory")
	assert.Equal(t, 422, res.StatusCode)
}

func TestCreateDirOnNonExistingParent(t *testing.T) {
	res, _ := createDir(t, "/files/noooop?Name=foo&Type=directory")
	assert.Equal(t, 404, res.StatusCode)
}

func TestCreateDirAlreadyExists(t *testing.T) {
	res1, _ := createDir(t, "/files/?Name=iexist&Type=directory")
	assert.Equal(t, 201, res1.StatusCode)

	res2, _ := createDir(t, "/files/?Name=iexist&Type=directory")
	assert.Equal(t, 409, res2.StatusCode)
}

func TestCreateDirRootSuccess(t *testing.T) {
	res, _ := createDir(t, "/files/?Name=coucou&Type=directory")
	assert.Equal(t, 201, res.StatusCode)

	storage := testInstance.VFS()
	exists, err := vfs.DirExists(storage, "/coucou")
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestCreateDirWithDateSuccess(t *testing.T) {
	req, _ := http.NewRequest("POST", ts.URL+"/files/?Type=directory&Name=dir-with-date", strings.NewReader(""))
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	req.Header.Add("Date", "Mon, 19 Sep 2016 12:35:08 GMT")
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 201, res.StatusCode)

	var obj map[string]interface{}
	err = extractJSONRes(res, &obj)
	assert.NoError(t, err)
	data := obj["data"].(map[string]interface{})
	attrs := data["attributes"].(map[string]interface{})
	createdAt := attrs["created_at"].(string)
	assert.Equal(t, "2016-09-19T12:35:08Z", createdAt)
	updatedAt := attrs["updated_at"].(string)
	assert.Equal(t, createdAt, updatedAt)
}

func TestCreateDirWithParentSuccess(t *testing.T) {
	res1, data1 := createDir(t, "/files/?Name=dirparent&Type=directory")
	assert.Equal(t, 201, res1.StatusCode)

	var ok bool
	data1, ok = data1["data"].(map[string]interface{})
	assert.True(t, ok)

	parentID, ok := data1["id"].(string)
	assert.True(t, ok)

	res2, _ := createDir(t, "/files/"+parentID+"?Name=child&Type=directory")
	assert.Equal(t, 201, res2.StatusCode)

	storage := testInstance.VFS()
	exists, err := vfs.DirExists(storage, "/dirparent/child")
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestCreateDirWithIllegalCharacter(t *testing.T) {
	res1, _ := createDir(t, "/files/?Name=coucou/les/copains!&Type=directory")
	assert.Equal(t, 422, res1.StatusCode)
}

func TestCreateDirConcurrently(t *testing.T) {
	done := make(chan *http.Response)
	errs := make(chan *http.Response)

	doCreateDir := func(name string) {
		res, _ := createDir(t, "/files/?Name="+name+"&Type=directory")
		if res.StatusCode == 201 {
			done <- res
		} else {
			errs <- res
		}
	}

	n := 100
	c := 0

	for i := 0; i < n; i++ {
		go doCreateDir("foo")
	}

	for i := 0; i < n; i++ {
		select {
		case res := <-errs:
			assert.True(t, res.StatusCode == 409 || res.StatusCode == 503)
		case <-done:
			c = c + 1
		}
	}

	assert.Equal(t, 1, c)
}

func TestUploadWithNoType(t *testing.T) {
	res, _ := upload(t, "/files/", "text/plain", "foo", "")
	assert.Equal(t, 422, res.StatusCode)
}

func TestUploadWithNoName(t *testing.T) {
	res, _ := upload(t, "/files/?Type=file", "text/plain", "foo", "")
	assert.Equal(t, 422, res.StatusCode)
}

func TestUploadToNonExistingParent(t *testing.T) {
	res, _ := upload(t, "/files/nooop?Type=file&Name=no-parent", "text/plain", "foo", "")
	assert.Equal(t, 404, res.StatusCode)
}

func TestUploadToTrashedFolder(t *testing.T) {
	res1, data1 := createDir(t, "/files/?Name=trashed-parent&Type=directory")
	assert.Equal(t, 201, res1.StatusCode)
	dirID, _ := extractDirData(t, data1)
	res2, _ := trash(t, "/files/"+dirID)
	assert.Equal(t, 200, res2.StatusCode)
	res3, _ := upload(t, "/files/"+dirID+"?Type=file&Name=trashed-parent", "text/plain", "foo", "")
	assert.Equal(t, 404, res3.StatusCode)
}

func TestUploadBadHash(t *testing.T) {
	body := "foo"
	res, _ := upload(t, "/files/?Type=file&Name=badhash", "text/plain", body, "3FbbMXfH+PdjAlWFfVb1dQ==")
	assert.Equal(t, 412, res.StatusCode)

	storage := testInstance.VFS()
	_, err := readFile(storage, "/badhash")
	assert.Error(t, err)
}

func TestUploadAtRootSuccess(t *testing.T) {
	body := "foo"
	res, _ := upload(t, "/files/?Type=file&Name=goodhash", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res.StatusCode)

	storage := testInstance.VFS()
	buf, err := readFile(storage, "/goodhash")
	assert.NoError(t, err)
	assert.Equal(t, body, string(buf))
}

func TestUploadImage(t *testing.T) {
	f, err := os.Open("../../tests/fixtures/wet-cozy_20160910__M4Dz.jpg")
	assert.NoError(t, err)
	defer f.Close()
	m := `{"gps":{"city":"Paris","country":"France"}}`
	req, err := http.NewRequest("POST", ts.URL+"/files/?Type=file&Name=wet.jpg&Metadata="+m, f)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	res, obj := doUploadOrMod(t, req, "image/jpeg", "tHWYYuXBBflJ8wXgJ2c2yg==")
	assert.Equal(t, 201, res.StatusCode)
	data := obj["data"].(map[string]interface{})
	imgID = data["id"].(string)
	attrs := data["attributes"].(map[string]interface{})
	meta := attrs["metadata"].(map[string]interface{})
	v := meta["extractor_version"].(float64)
	assert.Equal(t, float64(vfs.MetadataExtractorVersion), v)
	flash := meta["flash"].(string)
	assert.Equal(t, "Off, Did not fire", flash)
	gps := meta["gps"].(map[string]interface{})
	assert.Equal(t, "Paris", gps["city"])
	assert.Equal(t, "France", gps["country"])
}

func TestUploadWithParentSuccess(t *testing.T) {
	res1, data1 := createDir(t, "/files/?Name=fileparent&Type=directory")
	assert.Equal(t, 201, res1.StatusCode)

	var ok bool
	data1, ok = data1["data"].(map[string]interface{})
	assert.True(t, ok)

	parentID, ok := data1["id"].(string)
	assert.True(t, ok)

	body := "foo"
	res2, _ := upload(t, "/files/"+parentID+"?Type=file&Name=goodhash", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res2.StatusCode)

	storage := testInstance.VFS()
	buf, err := readFile(storage, "/fileparent/goodhash")
	assert.NoError(t, err)
	assert.Equal(t, body, string(buf))
}

func TestUploadAtRootAlreadyExists(t *testing.T) {
	body := "foo"
	res1, _ := upload(t, "/files/?Type=file&Name=iexistfile", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res1.StatusCode)

	res2, _ := upload(t, "/files/?Type=file&Name=iexistfile", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 409, res2.StatusCode)
}

func TestUploadWithParentAlreadyExists(t *testing.T) {
	_, dirdata := createDir(t, "/files/?Type=directory&Name=container")

	var ok bool
	dirdata, ok = dirdata["data"].(map[string]interface{})
	assert.True(t, ok)

	parentID, ok := dirdata["id"].(string)
	assert.True(t, ok)

	body := "foo"
	res1, _ := upload(t, "/files/"+parentID+"?Type=file&Name=iexistfile", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res1.StatusCode)

	res2, _ := upload(t, "/files/"+parentID+"?Type=file&Name=iexistfile", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 409, res2.StatusCode)
}

func TestUploadWithDate(t *testing.T) {
	buf := strings.NewReader("foo")
	req, err := http.NewRequest("POST", ts.URL+"/files/?Type=file&Name=withcdate", buf)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	assert.NoError(t, err)
	req.Header.Add("Date", "Mon, 19 Sep 2016 12:38:04 GMT")
	res, obj := doUploadOrMod(t, req, "text/plain", "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res.StatusCode)
	data := obj["data"].(map[string]interface{})
	attrs := data["attributes"].(map[string]interface{})
	createdAt := attrs["created_at"].(string)
	assert.Equal(t, "2016-09-19T12:38:04Z", createdAt)
	updatedAt := attrs["updated_at"].(string)
	assert.Equal(t, createdAt, updatedAt)
}

func TestModifyMetadataFileMove(t *testing.T) {
	body := "foo"
	res1, data1 := upload(t, "/files/?Type=file&Name=filemoveme&Tags=foo,bar", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res1.StatusCode)

	var ok bool
	data1, ok = data1["data"].(map[string]interface{})
	assert.True(t, ok)

	fileID, ok := data1["id"].(string)
	assert.True(t, ok)

	res2, data2 := createDir(t, "/files/?Name=movemeinme&Type=directory")
	assert.Equal(t, 201, res2.StatusCode)

	data2, ok = data2["data"].(map[string]interface{})
	assert.True(t, ok)

	dirID, ok := data2["id"].(string)
	assert.True(t, ok)

	attrs := map[string]interface{}{
		"tags":       []string{"bar", "bar", "baz"},
		"name":       "moved",
		"dir_id":     dirID,
		"executable": true,
	}

	res3, data3 := patchFile(t, "/files/"+fileID, "file", fileID, attrs, nil)
	assert.Equal(t, 200, res3.StatusCode)

	data3, ok = data3["data"].(map[string]interface{})
	assert.True(t, ok)

	attrs3, ok := data3["attributes"].(map[string]interface{})
	assert.True(t, ok)

	assert.Equal(t, "text/plain", attrs3["mime"])
	assert.Equal(t, "moved", attrs3["name"])
	assert.EqualValues(t, []interface{}{"bar", "baz"}, attrs3["tags"])
	assert.Equal(t, "text", attrs3["class"])
	assert.Equal(t, "rL0Y20zC+Fzt72VPzMSk2A==", attrs3["md5sum"])
	assert.Equal(t, true, attrs3["executable"])
	assert.Equal(t, "3", attrs3["size"])
}

func TestModifyMetadataFileConflict(t *testing.T) {
	body := "foo"
	res1, data1 := upload(t, "/files/?Type=file&Name=fmodme1&Tags=foo,bar", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res1.StatusCode)

	res2, _ := upload(t, "/files/?Type=file&Name=fmodme2&Tags=foo,bar", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res2.StatusCode)

	file1ID, _ := extractDirData(t, data1)

	attrs := map[string]interface{}{
		"name": "fmodme2",
	}

	res3, _ := patchFile(t, "/files/"+file1ID, "file", file1ID, attrs, nil)
	assert.Equal(t, 409, res3.StatusCode)
}

func TestModifyMetadataDirMove(t *testing.T) {
	res1, data1 := createDir(t, "/files/?Name=dirmodme&Type=directory&Tags=foo,bar,bar")
	assert.Equal(t, 201, res1.StatusCode)

	dir1ID, _ := extractDirData(t, data1)

	reschild1, _ := createDir(t, "/files/"+dir1ID+"?Name=child1&Type=directory")
	assert.Equal(t, 201, reschild1.StatusCode)

	reschild2, _ := createDir(t, "/files/"+dir1ID+"?Name=child2&Type=directory")
	assert.Equal(t, 201, reschild2.StatusCode)

	res2, data2 := createDir(t, "/files/?Name=dirmodmemoveinme&Type=directory")
	assert.Equal(t, 201, res2.StatusCode)

	dir2ID, _ := extractDirData(t, data2)

	attrs1 := map[string]interface{}{
		"tags":   []string{"bar", "baz"},
		"name":   "renamed",
		"dir_id": dir2ID,
	}

	res3, _ := patchFile(t, "/files/"+dir1ID, "directory", dir1ID, attrs1, nil)
	assert.Equal(t, 200, res3.StatusCode)

	storage := testInstance.VFS()
	exists, err := vfs.DirExists(storage, "/dirmodmemoveinme/renamed")
	assert.NoError(t, err)
	assert.True(t, exists)

	attrs2 := map[string]interface{}{
		"tags":   []string{"bar", "baz"},
		"name":   "renamed",
		"dir_id": dir1ID,
	}

	res4, _ := patchFile(t, "/files/"+dir2ID, "directory", dir2ID, attrs2, nil)
	assert.Equal(t, 412, res4.StatusCode)

	res5, _ := patchFile(t, "/files/"+dir1ID, "directory", dir1ID, attrs2, nil)
	assert.Equal(t, 412, res5.StatusCode)
}

func TestModifyMetadataDirMoveWithRel(t *testing.T) {
	res1, data1 := createDir(t, "/files/?Name=dirmodmewithrel&Type=directory&Tags=foo,bar,bar")
	assert.Equal(t, 201, res1.StatusCode)

	dir1ID, _ := extractDirData(t, data1)

	reschild1, datachild1 := createDir(t, "/files/"+dir1ID+"?Name=child1&Type=directory")
	assert.Equal(t, 201, reschild1.StatusCode)

	reschild2, datachild2 := createDir(t, "/files/"+dir1ID+"?Name=child2&Type=directory")
	assert.Equal(t, 201, reschild2.StatusCode)

	res2, data2 := createDir(t, "/files/?Name=dirmodmemoveinmewithrel&Type=directory")
	assert.Equal(t, 201, res2.StatusCode)

	dir2ID, _ := extractDirData(t, data2)
	child1ID, _ := extractDirData(t, datachild1)
	child2ID, _ := extractDirData(t, datachild2)

	fmt.Println(child1ID, child2ID)

	parent := &jsonData{
		ID:   dir2ID,
		Type: "io.cozy.files",
	}

	res3, _ := patchFile(t, "/files/"+dir1ID, "directory", dir1ID, nil, parent)
	assert.Equal(t, 200, res3.StatusCode)

	storage := testInstance.VFS()
	exists, err := vfs.DirExists(storage, "/dirmodmemoveinmewithrel/dirmodmewithrel")
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestModifyMetadataDirMoveConflict(t *testing.T) {
	res1, _ := createDir(t, "/files/?Name=conflictmodme1&Type=directory&Tags=foo,bar,bar")
	assert.Equal(t, 201, res1.StatusCode)

	res2, data2 := createDir(t, "/files/?Name=conflictmodme2&Type=directory")
	assert.Equal(t, 201, res2.StatusCode)

	dir2ID, _ := extractDirData(t, data2)

	attrs1 := map[string]interface{}{
		"tags": []string{"bar", "baz"},
		"name": "conflictmodme1",
	}

	res3, _ := patchFile(t, "/files/"+dir2ID, "directory", dir2ID, attrs1, nil)
	assert.Equal(t, 409, res3.StatusCode)
}

func TestModifyContentNoFileID(t *testing.T) {
	res, _ := uploadMod(t, "/files/badid", "text/plain", "nil", "")
	assert.Equal(t, 404, res.StatusCode)
}

func TestModifyContentBadRev(t *testing.T) {
	res1, data1 := upload(t, "/files/?Type=file&Name=modbadrev&Executable=true", "text/plain", "foo", "")
	assert.Equal(t, 201, res1.StatusCode)

	var ok bool
	data1, ok = data1["data"].(map[string]interface{})
	assert.True(t, ok)

	meta1, ok := data1["meta"].(map[string]interface{})
	assert.True(t, ok)

	fileID, ok := data1["id"].(string)
	assert.True(t, ok)
	fileRev, ok := meta1["rev"].(string)
	assert.True(t, ok)

	newcontent := "newcontent :)"

	req2, err := http.NewRequest("PUT", ts.URL+"/files/"+fileID, strings.NewReader(newcontent))
	req2.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	assert.NoError(t, err)

	req2.Header.Add("If-Match", "badrev")
	res2, _ := doUploadOrMod(t, req2, "text/plain", "")
	assert.Equal(t, 412, res2.StatusCode)

	req3, err := http.NewRequest("PUT", ts.URL+"/files/"+fileID, strings.NewReader(newcontent))
	req3.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	assert.NoError(t, err)

	req3.Header.Add("If-Match", fileRev)
	res3, _ := doUploadOrMod(t, req3, "text/plain", "")
	assert.Equal(t, 200, res3.StatusCode)
}

func TestModifyContentSuccess(t *testing.T) {
	var err error
	var buf []byte
	var fileInfo os.FileInfo

	storage := testInstance.VFS()
	res1, data1 := upload(t, "/files/?Type=file&Name=willbemodified&Executable=true", "text/plain", "foo", "")
	assert.Equal(t, 201, res1.StatusCode)

	buf, err = readFile(storage, "/willbemodified")
	assert.NoError(t, err)
	assert.Equal(t, "foo", string(buf))
	fileInfo, err = storage.FileByPath("/willbemodified")
	assert.NoError(t, err)
	assert.Equal(t, fileInfo.Mode().String(), "-rwxr-xr-x")

	var ok bool
	data1, ok = data1["data"].(map[string]interface{})
	assert.True(t, ok)

	attrs1, ok := data1["attributes"].(map[string]interface{})
	assert.True(t, ok)

	links1, ok := data1["links"].(map[string]interface{})
	assert.True(t, ok)

	fileID, ok := data1["id"].(string)
	assert.True(t, ok)

	newcontent := "newcontent :)"
	res2, data2 := uploadMod(t, "/files/"+fileID+"?Executable=false", "audio/mp3", newcontent, "")
	assert.Equal(t, 200, res2.StatusCode)

	data2, ok = data2["data"].(map[string]interface{})
	assert.True(t, ok)

	meta2, ok := data2["meta"].(map[string]interface{})
	assert.True(t, ok)

	attrs2, ok := data2["attributes"].(map[string]interface{})
	assert.True(t, ok)

	links2, ok := data2["links"].(map[string]interface{})
	assert.True(t, ok)

	assert.Equal(t, data2["id"], data1["id"], "same id")
	assert.Equal(t, data2["path"], data1["path"], "same path")
	assert.NotEqual(t, meta2["rev"], data1["rev"], "different rev")
	assert.Equal(t, links2["self"], links1["self"], "same self link")

	assert.Equal(t, attrs2["name"], attrs1["name"])
	assert.Equal(t, attrs2["created_at"], attrs1["created_at"])
	assert.NotEqual(t, attrs2["updated_at"], attrs1["updated_at"])
	assert.NotEqual(t, attrs2["size"], attrs1["size"])

	assert.Equal(t, attrs2["size"], strconv.Itoa(len(newcontent)))
	assert.NotEqual(t, attrs2["md5sum"], attrs1["md5sum"])
	assert.NotEqual(t, attrs2["class"], attrs1["class"])
	assert.NotEqual(t, attrs2["mime"], attrs1["mime"])
	assert.NotEqual(t, attrs2["executable"], attrs1["executable"])
	assert.Equal(t, attrs2["class"], "audio")
	assert.Equal(t, attrs2["mime"], "audio/mp3")
	assert.Equal(t, attrs2["executable"], false)

	buf, err = readFile(storage, "/willbemodified")
	assert.NoError(t, err)
	assert.Equal(t, newcontent, string(buf))
	fileInfo, err = storage.FileByPath("/willbemodified")
	assert.NoError(t, err)
	assert.Equal(t, fileInfo.Mode().String(), "-rw-r--r--")

	req, err := http.NewRequest("PUT", ts.URL+"/files/"+fileID, strings.NewReader(""))
	assert.NoError(t, err)

	req.Header.Add("Date", "Mon, 02 Jan 2006 15:04:05 MST")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)

	res3, data3 := doUploadOrMod(t, req, "what/ever", "")
	assert.Equal(t, 200, res3.StatusCode)

	data3, ok = data3["data"].(map[string]interface{})
	assert.True(t, ok)

	attrs3, ok := data3["attributes"].(map[string]interface{})
	assert.True(t, ok)

	assert.Equal(t, "2006-01-02T15:04:05Z", attrs3["updated_at"])
}

func TestModifyContentConcurrently(t *testing.T) {
	type result struct {
		rev string
		idx int64
	}

	done := make(chan *result)
	errs := make(chan *http.Response)

	res, data := upload(t, "/files/?Type=file&Name=willbemodifiedconcurrently&Executable=true", "text/plain", "foo", "")
	if !assert.Equal(t, 201, res.StatusCode) {
		return
	}

	var ok bool
	data, ok = data["data"].(map[string]interface{})
	assert.True(t, ok)

	fileID, ok := data["id"].(string)
	assert.True(t, ok)

	var c int64

	doModContent := func() {
		idx := atomic.AddInt64(&c, 1)
		res, data := uploadMod(t, "/files/"+fileID, "plain/text", "newcontent "+strconv.FormatInt(idx, 10), "")
		if res.StatusCode == 200 {
			data = data["data"].(map[string]interface{})
			meta := data["meta"].(map[string]interface{})
			done <- &result{meta["rev"].(string), idx}
		} else {
			errs <- res
		}
	}

	n := 100

	for i := 0; i < n; i++ {
		go doModContent()
	}

	var successes []*result
	for i := 0; i < n; i++ {
		select {
		case res := <-errs:
			assert.True(t, res.StatusCode == 409 || res.StatusCode == 503, "status code is %v and not 409 or 503", res.StatusCode)
		case res := <-done:
			successes = append(successes, res)
		}
	}

	assert.True(t, len(successes) >= 1, "there is at least one success")

	for i, s := range successes {
		assert.True(t, strings.HasPrefix(s.rev, strconv.Itoa(i+3)+"-"))
	}

	storage := testInstance.VFS()
	buf, err := readFile(storage, "/willbemodifiedconcurrently")
	assert.NoError(t, err)

	found := false
	for _, s := range successes {
		if string(buf) == "newcontent "+strconv.FormatInt(s.idx, 10) {
			found = true
			break
		}
	}

	assert.True(t, found)
}

func TestDownloadFileBadID(t *testing.T) {
	res, _ := download(t, "/files/download/badid", "")
	assert.Equal(t, 404, res.StatusCode)
}

func TestDownloadFileBadPath(t *testing.T) {
	res, _ := download(t, "/files/download?Path=/i/do/not/exist", "")
	assert.Equal(t, 404, res.StatusCode)
}

func TestDownloadFileByIDSuccess(t *testing.T) {
	body := "foo"
	res1, filedata := upload(t, "/files/?Type=file&Name=downloadme1", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res1.StatusCode)

	var ok bool
	filedata, ok = filedata["data"].(map[string]interface{})
	assert.True(t, ok)

	fileID, ok := filedata["id"].(string)
	assert.True(t, ok)

	res2, resbody := download(t, "/files/download/"+fileID, "")
	assert.Equal(t, 200, res2.StatusCode)
	assert.True(t, strings.HasPrefix(res2.Header.Get("Content-Disposition"), "inline"))
	assert.True(t, strings.Contains(res2.Header.Get("Content-Disposition"), `filename="downloadme1"`))
	assert.True(t, strings.HasPrefix(res2.Header.Get("Content-Type"), "text/plain"))
	assert.NotEmpty(t, res2.Header.Get("Etag"))
	assert.Equal(t, res2.Header.Get("Etag")[:1], `"`)
	assert.Equal(t, res2.Header.Get("Content-Length"), "3")
	assert.Equal(t, res2.Header.Get("Accept-Ranges"), "bytes")
	assert.Equal(t, body, string(resbody))
}

func TestDownloadFileByPathSuccess(t *testing.T) {
	body := "foo"
	res1, _ := upload(t, "/files/?Type=file&Name=downloadme2", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res1.StatusCode)

	res2, resbody := download(t, "/files/download?Dl=1&Path="+url.QueryEscape("/downloadme2"), "")
	assert.Equal(t, 200, res2.StatusCode)
	assert.True(t, strings.HasPrefix(res2.Header.Get("Content-Disposition"), "attachment"))
	assert.True(t, strings.Contains(res2.Header.Get("Content-Disposition"), `filename="downloadme2"`))
	assert.True(t, strings.HasPrefix(res2.Header.Get("Content-Type"), "text/plain"))
	assert.Equal(t, res2.Header.Get("Content-Length"), "3")
	assert.Equal(t, res2.Header.Get("Accept-Ranges"), "bytes")
	assert.Equal(t, body, string(resbody))
}

func TestDownloadRangeSuccess(t *testing.T) {
	body := "foo,bar"
	res1, _ := upload(t, "/files/?Type=file&Name=downloadmebyrange", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	assert.Equal(t, 201, res1.StatusCode)

	res2, _ := download(t, "/files/download?Path="+url.QueryEscape("/downloadmebyrange"), "nimp")
	assert.Equal(t, 416, res2.StatusCode)

	res3, res3body := download(t, "/files/download?Path="+url.QueryEscape("/downloadmebyrange"), "bytes=0-2")
	assert.Equal(t, 206, res3.StatusCode)
	assert.Equal(t, "foo", string(res3body))

	res4, res4body := download(t, "/files/download?Path="+url.QueryEscape("/downloadmebyrange"), "bytes=4-")
	assert.Equal(t, 206, res4.StatusCode)
	assert.Equal(t, "bar", string(res4body))
}

func TestGetFileMetadataFromPath(t *testing.T) {
	res1, _ := httpGet(ts.URL + "/files/metadata?Path=/noooooop")
	assert.Equal(t, 404, res1.StatusCode)

	body := "foo,bar"
	res2, _ := upload(t, "/files/?Type=file&Name=getmetadata", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	assert.Equal(t, 201, res2.StatusCode)

	res3, _ := httpGet(ts.URL + "/files/metadata?Path=/getmetadata")
	assert.Equal(t, 200, res3.StatusCode)
}

func TestGetDirMetadataFromPath(t *testing.T) {
	res1, _ := createDir(t, "/files/?Name=getdirmeta&Type=directory")
	assert.Equal(t, 201, res1.StatusCode)

	res2, _ := httpGet(ts.URL + "/files/metadata?Path=/getdirmeta")
	assert.Equal(t, 200, res2.StatusCode)
}

func TestGetFileMetadataFromID(t *testing.T) {
	res1, _ := httpGet(ts.URL + "/files/qsdqsd")
	assert.Equal(t, 404, res1.StatusCode)

	body := "foo,bar"
	res2, data2 := upload(t, "/files/?Type=file&Name=getmetadatafromid", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	assert.Equal(t, 201, res2.StatusCode)

	fileID, _ := extractDirData(t, data2)

	res3, _ := httpGet(ts.URL + "/files/" + fileID)
	assert.Equal(t, 200, res3.StatusCode)
}

func TestGetDirMetadataFromID(t *testing.T) {
	res1, data1 := createDir(t, "/files/?Name=getdirmetafromid&Type=directory")
	assert.Equal(t, 201, res1.StatusCode)

	parentID, _ := extractDirData(t, data1)

	body := "foo"
	res2, data2 := upload(t, "/files/"+parentID+"?Type=file&Name=firstfile", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res2.StatusCode)

	fileID, _ := extractDirData(t, data2)

	res3, _ := httpGet(ts.URL + "/files/" + fileID)
	assert.Equal(t, 200, res3.StatusCode)
}

func TestArchiveNoFiles(t *testing.T) {
	body := bytes.NewBufferString(`{
		"data": {
			"attributes": {}
		}
	}`)
	req, err := http.NewRequest("POST", ts.URL+"/files/archive", body)
	if !assert.NoError(t, err) {
		return
	}
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}

	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)
	msg, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.Equal(t, `"Can't create an archive with no files"`, string(msg))
}

func TestArchiveDirectDownload(t *testing.T) {
	res1, data1 := createDir(t, "/files/?Name=archive&Type=directory")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}
	dirID, _ := extractDirData(t, data1)
	names := []string{"foo", "bar", "baz"}
	for _, name := range names {
		res2, _ := createDir(t, "/files/"+dirID+"?Name="+name+".jpg&Type=file")
		if !assert.Equal(t, 201, res2.StatusCode) {
			return
		}
	}

	// direct download
	body := bytes.NewBufferString(`{
		"data": {
			"attributes": {
				"files": [
					"/archive/foo.jpg",
					"/archive/bar.jpg",
					"/archive/baz.jpg"
				]
			}
		}
	}`)

	req, err := http.NewRequest("POST", ts.URL+"/files/archive", body)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	req.Header.Add("Content-Type", "application/zip")
	req.Header.Add("Accept", "application/zip")
	assert.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	assert.Equal(t, "application/zip", res.Header.Get("Content-Type"))

}

func TestArchiveCreateAndDownload(t *testing.T) {
	res1, data1 := createDir(t, "/files/?Name=archive2&Type=directory")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}
	dirID, _ := extractDirData(t, data1)
	names := []string{"foo", "bar", "baz"}
	for _, name := range names {
		res2, _ := createDir(t, "/files/"+dirID+"?Name="+name+".jpg&Type=file")
		if !assert.Equal(t, 201, res2.StatusCode) {
			return
		}
	}

	body := bytes.NewBufferString(`{
		"data": {
			"attributes": {
				"files": [
					"/archive/foo.jpg",
					"/archive/bar.jpg",
					"/archive/baz.jpg"
				]
			}
		}
	}`)

	req, err := http.NewRequest("POST", ts.URL+"/files/archive", body)
	if !assert.NoError(t, err) {
		return
	}
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}

	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	var data map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&data)
	assert.NoError(t, err)

	downloadURL := ts.URL + data["links"].(map[string]interface{})["related"].(string)
	res2, err := httpGet(downloadURL)
	assert.NoError(t, err)
	assert.Equal(t, 200, res2.StatusCode)
	disposition := res2.Header.Get("Content-Disposition")
	assert.Equal(t, `attachment; filename="archive.zip"`, disposition)
}

func TestFileCreateAndDownloadByPath(t *testing.T) {
	body := "foo,bar"
	res1, _ := upload(t, "/files/?Type=file&Name=todownload2steps", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	path := "/todownload2steps"

	req, err := http.NewRequest("POST", ts.URL+"/files/downloads?Path="+path, nil)
	if !assert.NoError(t, err) {
		return
	}
	req.Header.Add("Content-Type", "")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}

	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	var data map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&data)
	assert.NoError(t, err)

	displayURL := ts.URL + data["links"].(map[string]interface{})["related"].(string)
	res2, err := http.Get(displayURL)
	assert.NoError(t, err)
	assert.Equal(t, 200, res2.StatusCode)
	disposition := res2.Header.Get("Content-Disposition")
	assert.Equal(t, `inline; filename="todownload2steps"`, disposition)

	downloadURL := ts.URL + data["links"].(map[string]interface{})["related"].(string) + "?Dl=1"
	res3, err := http.Get(downloadURL)
	assert.NoError(t, err)
	assert.Equal(t, 200, res3.StatusCode)
	disposition = res3.Header.Get("Content-Disposition")
	assert.Equal(t, `attachment; filename="todownload2steps"`, disposition)
}

func TestFileCreateAndDownloadByID(t *testing.T) {
	body := "foo,bar"
	res1, v := upload(t, "/files/?Type=file&Name=todownload2stepsbis", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}
	id := v["data"].(map[string]interface{})["id"].(string)

	req, err := http.NewRequest("POST", ts.URL+"/files/downloads?Id="+id, nil)
	if !assert.NoError(t, err) {
		return
	}
	req.Header.Add("Content-Type", "")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}

	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	var data map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&data)
	assert.NoError(t, err)

	displayURL := ts.URL + data["links"].(map[string]interface{})["related"].(string)
	res2, err := http.Get(displayURL)
	assert.NoError(t, err)
	assert.Equal(t, 200, res2.StatusCode)
	disposition := res2.Header.Get("Content-Disposition")
	assert.Equal(t, `inline; filename="todownload2stepsbis"`, disposition)
}

func TestHeadDirOrFileNotFound(t *testing.T) {
	req, _ := http.NewRequest("HEAD", ts.URL+"/files/fakeid/?Type=directory", strings.NewReader(""))
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestHeadDirOrFileExists(t *testing.T) {
	res, _ := createDir(t, "/files/?Name=hellothere&Type=directory")
	assert.Equal(t, 201, res.StatusCode)

	storage := testInstance.VFS()
	dir, err := storage.DirByPath("/hellothere")
	assert.NoError(t, err)
	id := dir.ID()
	req, _ := http.NewRequest("HEAD", ts.URL+"/files/"+id+"?Type=directory", strings.NewReader(""))
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	res, err = http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
}

func TestArchiveNotFound(t *testing.T) {
	body := bytes.NewBufferString(`{
		"data": {
			"attributes": {
				"files": [
					"/archive/foo.jpg",
					"/no/such/file",
					"/archive/baz.jpg"
				]
			}
		}
	}`)
	req, err := http.NewRequest("POST", ts.URL+"/files/archive", body)
	if !assert.NoError(t, err) {
		return
	}
	req.Header.Add("Content-Type", "application/vnd.api+json")
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, 404, res.StatusCode)
}

func TestDirTrash(t *testing.T) {
	res1, data1 := createDir(t, "/files/?Name=totrashdir&Type=directory")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	dirID, _ := extractDirData(t, data1)

	res2, _ := createDir(t, "/files/"+dirID+"?Name=child1&Type=file")
	if !assert.Equal(t, 201, res2.StatusCode) {
		return
	}
	res3, _ := createDir(t, "/files/"+dirID+"?Name=child2&Type=file")
	if !assert.Equal(t, 201, res3.StatusCode) {
		return
	}

	res4, _ := trash(t, "/files/"+dirID)
	if !assert.Equal(t, 200, res4.StatusCode) {
		return
	}

	res5, err := httpGet(ts.URL + "/files/" + dirID)
	if !assert.NoError(t, err) || !assert.Equal(t, 200, res5.StatusCode) {
		return
	}

	res6, err := httpGet(ts.URL + "/files/download?Path=" + url.QueryEscape(vfs.TrashDirName+"/totrashdir/child1"))
	if !assert.NoError(t, err) || !assert.Equal(t, 200, res6.StatusCode) {
		return
	}

	res7, err := httpGet(ts.URL + "/files/download?Path=" + url.QueryEscape(vfs.TrashDirName+"/totrashdir/child2"))
	if !assert.NoError(t, err) || !assert.Equal(t, 200, res7.StatusCode) {
		return
	}

	res8, _ := trash(t, "/files/"+dirID)
	if !assert.Equal(t, 400, res8.StatusCode) {
		return
	}
}

func TestFileTrash(t *testing.T) {
	body := "foo,bar"
	res1, data1 := upload(t, "/files/?Type=file&Name=totrashfile", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	fileID, _ := extractDirData(t, data1)

	res2, _ := trash(t, "/files/"+fileID)
	if !assert.Equal(t, 200, res2.StatusCode) {
		return
	}

	res3, err := httpGet(ts.URL + "/files/download?Path=" + url.QueryEscape(vfs.TrashDirName+"/totrashfile"))
	if !assert.NoError(t, err) || !assert.Equal(t, 200, res3.StatusCode) {
		return
	}

	res4, _ := trash(t, "/files/"+fileID)
	if !assert.Equal(t, 400, res4.StatusCode) {
		return
	}

	res5, data2 := upload(t, "/files/?Type=file&Name=totrashfile2", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	if !assert.Equal(t, 201, res5.StatusCode) {
		return
	}

	fileID, v2 := extractDirData(t, data2)
	meta2 := v2["meta"].(map[string]interface{})
	rev2 := meta2["rev"].(string)

	req6, err := http.NewRequest("DELETE", ts.URL+"/files/"+fileID, nil)
	req6.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	assert.NoError(t, err)

	req6.Header.Add("If-Match", "badrev")
	res6, err := http.DefaultClient.Do(req6)
	assert.NoError(t, err)
	assert.Equal(t, 412, res6.StatusCode)

	res7, err := httpGet(ts.URL + "/files/download?Path=" + url.QueryEscape(vfs.TrashDirName+"/totrashfile"))
	if !assert.NoError(t, err) || !assert.Equal(t, 200, res7.StatusCode) {
		return
	}

	req8, err := http.NewRequest("DELETE", ts.URL+"/files/"+fileID, nil)
	req8.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	assert.NoError(t, err)

	req8.Header.Add("If-Match", rev2)
	res8, err := http.DefaultClient.Do(req8)
	assert.NoError(t, err)
	assert.Equal(t, 200, res8.StatusCode)

	res9, _ := trash(t, "/files/"+fileID)
	if !assert.Equal(t, 400, res9.StatusCode) {
		return
	}
}

func TestFileRestore(t *testing.T) {
	body := "foo,bar"
	res1, data1 := upload(t, "/files/?Type=file&Name=torestorefile", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	fileID, _ := extractDirData(t, data1)

	res2, body2 := trash(t, "/files/"+fileID)
	if !assert.Equal(t, 200, res2.StatusCode) {
		return
	}
	data2 := body2["data"].(map[string]interface{})
	attrs2 := data2["attributes"].(map[string]interface{})
	trashed := attrs2["trashed"].(bool)
	assert.True(t, trashed)

	res3, body3 := restore(t, "/files/trash/"+fileID)
	if !assert.Equal(t, 200, res3.StatusCode) {
		return
	}
	data3 := body3["data"].(map[string]interface{})
	attrs3 := data3["attributes"].(map[string]interface{})
	trashed = attrs3["trashed"].(bool)
	assert.False(t, trashed)

	res4, err := httpGet(ts.URL + "/files/download?Path=" + url.QueryEscape("/torestorefile"))
	if !assert.NoError(t, err) || !assert.Equal(t, 200, res4.StatusCode) {
		return
	}
}

func TestFileRestoreWithConflicts(t *testing.T) {
	body := "foo,bar"
	res1, data1 := upload(t, "/files/?Type=file&Name=torestorefilewithconflict", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	fileID, _ := extractDirData(t, data1)

	res2, _ := trash(t, "/files/"+fileID)
	if !assert.Equal(t, 200, res2.StatusCode) {
		return
	}

	res1, _ = upload(t, "/files/?Type=file&Name=torestorefilewithconflict", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	res3, data3 := restore(t, "/files/trash/"+fileID)
	if !assert.Equal(t, 200, res3.StatusCode) {
		return
	}

	restoredID, restoredData := extractDirData(t, data3)
	if !assert.Equal(t, fileID, restoredID) {
		return
	}
	restoredData = restoredData["attributes"].(map[string]interface{})
	assert.True(t, strings.HasPrefix(restoredData["name"].(string), "torestorefilewithconflict"))
	assert.NotEqual(t, "torestorefilewithconflict", restoredData["name"].(string))
}

func TestFileRestoreWithWithoutParent(t *testing.T) {
	res1, data1 := createDir(t, "/files/?Type=directory&Name=torestorein")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	dirID, _ := extractDirData(t, data1)

	body := "foo,bar"
	res1, data1 = upload(t, "/files/"+dirID+"?Type=file&Name=torestorefilewithconflict", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	fileID, _ := extractDirData(t, data1)

	res2, _ := trash(t, "/files/"+fileID)
	if !assert.Equal(t, 200, res2.StatusCode) {
		return
	}

	res2, _ = trash(t, "/files/"+dirID)
	if !assert.Equal(t, 200, res2.StatusCode) {
		return
	}

	res3, data3 := restore(t, "/files/trash/"+fileID)
	if !assert.Equal(t, 200, res3.StatusCode) {
		return
	}

	restoredID, restoredData := extractDirData(t, data3)
	if !assert.Equal(t, fileID, restoredID) {
		return
	}
	restoredData = restoredData["attributes"].(map[string]interface{})
	assert.Equal(t, "torestorefilewithconflict", restoredData["name"].(string))
	assert.NotEqual(t, consts.RootDirID, restoredData["dir_id"].(string))
}

func TestFileRestoreWithWithoutParent2(t *testing.T) {
	res1, data1 := createDir(t, "/files/?Type=directory&Name=torestorein2")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	dirID, _ := extractDirData(t, data1)

	body := "foo,bar"
	res1, data1 = upload(t, "/files/"+dirID+"?Type=file&Name=torestorefilewithconflict2", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	fileID, _ := extractDirData(t, data1)

	res2, _ := trash(t, "/files/"+dirID)
	if !assert.Equal(t, 200, res2.StatusCode) {
		return
	}

	res3, data3 := restore(t, "/files/trash/"+fileID)
	if !assert.Equal(t, 200, res3.StatusCode) {
		return
	}

	restoredID, restoredData := extractDirData(t, data3)
	if !assert.Equal(t, fileID, restoredID) {
		return
	}
	restoredData = restoredData["attributes"].(map[string]interface{})
	assert.Equal(t, "torestorefilewithconflict2", restoredData["name"].(string))
	assert.NotEqual(t, consts.RootDirID, restoredData["dir_id"].(string))
}

func TestDirRestore(t *testing.T) {
	res1, data1 := createDir(t, "/files/?Type=directory&Name=torestoredir")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	dirID, _ := extractDirData(t, data1)

	body := "foo,bar"
	res2, data2 := upload(t, "/files/"+dirID+"?Type=file&Name=totrashfile", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	if !assert.Equal(t, 201, res2.StatusCode) {
		return
	}

	fileID, _ := extractDirData(t, data2)

	res3, _ := trash(t, "/files/"+dirID)
	if !assert.Equal(t, 200, res3.StatusCode) {
		return
	}

	res4, err := httpGet(ts.URL + "/files/" + fileID)
	if !assert.NoError(t, err) || !assert.Equal(t, 200, res4.StatusCode) {
		return
	}

	var v map[string]interface{}
	err = extractJSONRes(res4, &v)
	assert.NoError(t, err)
	data := v["data"].(map[string]interface{})
	attrs := data["attributes"].(map[string]interface{})
	trashed := attrs["trashed"].(bool)
	assert.True(t, trashed)

	res5, _ := restore(t, "/files/trash/"+dirID)
	if !assert.Equal(t, 200, res5.StatusCode) {
		return
	}

	res6, err := httpGet(ts.URL + "/files/" + fileID)
	if !assert.NoError(t, err) || !assert.Equal(t, 200, res6.StatusCode) {
		return
	}

	err = extractJSONRes(res6, &v)
	assert.NoError(t, err)
	data = v["data"].(map[string]interface{})
	attrs = data["attributes"].(map[string]interface{})
	trashed = attrs["trashed"].(bool)
	assert.False(t, trashed)
}

func TestDirRestoreWithConflicts(t *testing.T) {
	res1, data1 := createDir(t, "/files/?Type=directory&Name=torestoredirwithconflict")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	dirID, _ := extractDirData(t, data1)

	res2, _ := trash(t, "/files/"+dirID)
	if !assert.Equal(t, 200, res2.StatusCode) {
		return
	}

	res1, _ = createDir(t, "/files/?Type=directory&Name=torestoredirwithconflict")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	res3, data3 := restore(t, "/files/trash/"+dirID)
	if !assert.Equal(t, 200, res3.StatusCode) {
		return
	}

	restoredID, restoredData := extractDirData(t, data3)
	if !assert.Equal(t, dirID, restoredID) {
		return
	}
	restoredData = restoredData["attributes"].(map[string]interface{})
	assert.True(t, strings.HasPrefix(restoredData["name"].(string), "torestoredirwithconflict"))
	assert.NotEqual(t, "torestoredirwithconflict", restoredData["name"].(string))
}

func TestTrashList(t *testing.T) {
	body := "foo,bar"
	res1, data1 := upload(t, "/files/?Type=file&Name=tolistfile", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	res2, data2 := createDir(t, "/files/?Name=tolistdir&Type=directory")
	if !assert.Equal(t, 201, res2.StatusCode) {
		return
	}

	dirID, _ := extractDirData(t, data1)
	fileID, _ := extractDirData(t, data2)

	res3, _ := trash(t, "/files/"+dirID)
	if !assert.Equal(t, 200, res3.StatusCode) {
		return
	}

	res4, _ := trash(t, "/files/"+fileID)
	if !assert.Equal(t, 200, res4.StatusCode) {
		return
	}

	res5, err := httpGet(ts.URL + "/files/trash")
	if !assert.NoError(t, err) {
		return
	}
	defer res5.Body.Close()

	var v struct {
		Data []interface{} `json:"data"`
	}

	err = json.NewDecoder(res5.Body).Decode(&v)
	if !assert.NoError(t, err) {
		return
	}

	assert.True(t, len(v.Data) >= 2, "response should contains at least 2 items")
}

func TestTrashClear(t *testing.T) {
	body := "foo,bar"
	res1, data1 := upload(t, "/files/?Type=file&Name=tolistfile", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	res2, data2 := createDir(t, "/files/?Name=tolistdir&Type=directory")
	if !assert.Equal(t, 201, res2.StatusCode) {
		return
	}

	dirID, _ := extractDirData(t, data1)
	fileID, _ := extractDirData(t, data2)

	res3, _ := trash(t, "/files/"+dirID)
	if !assert.Equal(t, 200, res3.StatusCode) {
		return
	}

	res4, _ := trash(t, "/files/"+fileID)
	if !assert.Equal(t, 200, res4.StatusCode) {
		return
	}

	path := "/files/trash"
	req, err := http.NewRequest(http.MethodDelete, ts.URL+path, nil)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	if !assert.NoError(t, err) {
		return
	}

	_, err = http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}

	res5, err := httpGet(ts.URL + "/files/trash")
	if !assert.NoError(t, err) {
		return
	}
	defer res5.Body.Close()

	var v struct {
		Data []interface{} `json:"data"`
	}

	err = json.NewDecoder(res5.Body).Decode(&v)
	if !assert.NoError(t, err) {
		return
	}

	assert.True(t, len(v.Data) == 0)
}

func TestDestroyFile(t *testing.T) {
	body := "foo,bar"
	res1, data1 := upload(t, "/files/?Type=file&Name=tolistfile", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	if !assert.Equal(t, 201, res1.StatusCode) {
		return
	}

	res2, data2 := createDir(t, "/files/?Name=tolistdir&Type=directory")
	if !assert.Equal(t, 201, res2.StatusCode) {
		return
	}

	dirID, _ := extractDirData(t, data1)
	fileID, _ := extractDirData(t, data2)

	res3, _ := trash(t, "/files/"+dirID)
	if !assert.Equal(t, 200, res3.StatusCode) {
		return
	}

	res4, _ := trash(t, "/files/"+fileID)
	if !assert.Equal(t, 200, res4.StatusCode) {
		return
	}

	path := "/files/trash/" + fileID
	req, err := http.NewRequest(http.MethodDelete, ts.URL+path, nil)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	if !assert.NoError(t, err) {
		return
	}

	_, err = http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}

	res5, err := httpGet(ts.URL + "/files/trash")
	if !assert.NoError(t, err) {
		return
	}
	defer res5.Body.Close()

	var v struct {
		Data []interface{} `json:"data"`
	}

	err = json.NewDecoder(res5.Body).Decode(&v)
	if !assert.NoError(t, err) {
		return
	}
	assert.True(t, len(v.Data) == 1)

	path = "/files/trash/" + dirID
	req, err = http.NewRequest(http.MethodDelete, ts.URL+path, nil)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	if !assert.NoError(t, err) {
		return
	}

	_, err = http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}

	res5, err = httpGet(ts.URL + "/files/trash")
	if !assert.NoError(t, err) {
		return
	}
	defer res5.Body.Close()

	err = json.NewDecoder(res5.Body).Decode(&v)
	if !assert.NoError(t, err) {
		return
	}
	assert.True(t, len(v.Data) == 0)
}

func TestThumbnail(t *testing.T) {
	res1, _ := httpGet(ts.URL + "/files/" + imgID)
	assert.Equal(t, 200, res1.StatusCode)
	var obj map[string]interface{}
	err := extractJSONRes(res1, &obj)
	assert.NoError(t, err)
	data := obj["data"].(map[string]interface{})
	links := data["links"].(map[string]interface{})
	large := links["large"].(string)
	medium := links["medium"].(string)
	small := links["small"].(string)

	res2, _ := download(t, large, "")
	assert.Equal(t, 200, res2.StatusCode)
	assert.True(t, strings.HasPrefix(res2.Header.Get("Content-Type"), "image/jpeg"))
	res3, _ := download(t, medium, "")
	assert.Equal(t, 200, res3.StatusCode)
	assert.True(t, strings.HasPrefix(res3.Header.Get("Content-Type"), "image/jpeg"))
	res4, _ := download(t, small, "")
	assert.Equal(t, 200, res4.StatusCode)
	assert.True(t, strings.HasPrefix(res4.Header.Get("Content-Type"), "image/jpeg"))
}

func TestGetFileByPublicLink(t *testing.T) {
	var err error
	body := "foo"
	res1, filedata := upload(t, "/files/?Type=file&Name=publicfile", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res1.StatusCode)

	var ok bool
	filedata, ok = filedata["data"].(map[string]interface{})
	assert.True(t, ok)

	fileID, ok = filedata["id"].(string)
	assert.True(t, ok)

	// Generating a new token
	publicToken, err = testInstance.MakeJWT(consts.ShareAudience, "email", "io.cozy.files", "", time.Now())
	assert.NoError(t, err)

	expires := time.Now().Add(2 * time.Minute)
	rules := permission.Set{
		permission.Rule{
			Type:   "io.cozy.files",
			Verbs:  permission.Verbs(permission.GET),
			Values: []string{fileID},
		},
	}
	_, err = permission.CreateShareSet(testInstance, &permission.Permission{Type: "app", Permissions: rules}, "", map[string]string{"email": publicToken}, nil, rules, &expires)
	assert.NoError(t, err)

	req, err := http.NewRequest("GET", ts.URL+"/files/"+fileID, nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+publicToken)
	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
}

func TestGetFileByPublicLinkRateExceeded(t *testing.T) {
	var err error
	// Blocking the file by accessing it a lot of times
	for i := 0; i < 1999; i++ {
		err = limits.CheckRateLimitKey(fileID, limits.SharingPublicLinkType)
		assert.NoError(t, err)
	}

	err = limits.CheckRateLimitKey(fileID, limits.SharingPublicLinkType)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Rate limit exceeded")
	req, err := http.NewRequest("GET", ts.URL+"/files/"+fileID, nil)
	assert.NoError(t, err)
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+publicToken)

	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 500, res.StatusCode)
	resbody, err := ioutil.ReadAll(res.Body)
	assert.NoError(t, err)
	assert.Contains(t, string(resbody), "Rate limit exceeded")
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	testutils.NeedCouchdb()
	setup := testutils.NewSetup(m, "files_test")

	tempdir, err := ioutil.TempDir("", "cozy-stack")
	if err != nil {
		fmt.Println("Could not create temporary directory.")
		os.Exit(1)
	}
	setup.AddCleanup(func() error { return os.RemoveAll(tempdir) })

	config.GetConfig().Fs.URL = &url.URL{
		Scheme: "file",
		Host:   "localhost",
		Path:   tempdir,
	}

	testInstance = setup.GetTestInstance()
	client, tok := setup.GetTestClient(consts.Files)
	clientID = client.ClientID
	token = tok
	ts = setup.GetTestServer("/files", Routes, func(r *echo.Echo) *echo.Echo {
		secure := middlewares.Secure(&middlewares.SecureConfig{
			CSPDefaultSrc:     []middlewares.CSPSource{middlewares.CSPSrcSelf},
			CSPFrameAncestors: []middlewares.CSPSource{middlewares.CSPSrcNone},
		})
		r.Use(secure)
		return r
	})
	ts.Config.Handler.(*echo.Echo).HTTPErrorHandler = errors.ErrorHandler

	os.Exit(setup.Run())
}

func httpGet(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	return http.DefaultClient.Do(req)
}
