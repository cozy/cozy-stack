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

	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/couchdb/mango"
	"github.com/cozy/cozy-stack/vfs"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
	"github.com/sourcegraph/checkup"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

const CouchURL = "http://localhost:5984/"
const TestPrefix = "test/"

var ts *httptest.Server
var instance *middlewares.Instance

func injectInstance(instance *middlewares.Instance) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("instance", instance)
	}
}

func extractJSONRes(res *http.Response, mp *map[string]interface{}) (err error) {
	if res.StatusCode >= 300 {
		return
	}

	var b []byte

	b, err = ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}

	err = json.Unmarshal(b, mp)
	return
}

func createDir(t *testing.T, path string) (res *http.Response, v map[string]interface{}) {
	res, err := http.Post(ts.URL+path, "text/plain", strings.NewReader(""))
	if !assert.NoError(t, err) {
		return
	}
	defer res.Body.Close()

	err = extractJSONRes(res, &v)
	assert.NoError(t, err)

	return
}

func doUploadOrMod(t *testing.T, req *http.Request, contentType, body, hash string) (res *http.Response, v map[string]interface{}) {
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
	if !assert.NoError(t, err) {
		return
	}
	return doUploadOrMod(t, req, contentType, body, hash)
}

func uploadMod(t *testing.T, path, contentType, body, hash string) (res *http.Response, v map[string]interface{}) {
	buf := strings.NewReader(body)
	req, err := http.NewRequest("PUT", ts.URL+path, buf)
	if !assert.NoError(t, err) {
		return
	}
	return doUploadOrMod(t, req, contentType, body, hash)
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

func patchFile(t *testing.T, path, docType, id string, attrs map[string]interface{}) (res *http.Response, v map[string]interface{}) {
	type jsonData struct {
		Type  string                 `json:"type"`
		ID    string                 `json:"id"`
		Attrs map[string]interface{} `json:"attributes,omitempty"`
	}

	bodyreq := &jsonData{
		Type:  docType,
		ID:    id,
		Attrs: attrs,
	}

	b, err := json.Marshal(map[string]*jsonData{"data": bodyreq})
	req, err := http.NewRequest("PATCH", ts.URL+path, bytes.NewReader(b))
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
	res, _ := createDir(t, "/files/?Type=io.cozy.folders")
	assert.Equal(t, 422, res.StatusCode)
}

func TestCreateDirOnNonExistingParent(t *testing.T) {
	res, _ := createDir(t, "/files/noooop?Name=foo&Type=io.cozy.folders")
	assert.Equal(t, 404, res.StatusCode)
}

func TestCreateDirAlreadyExists(t *testing.T) {
	res1, _ := createDir(t, "/files/?Name=iexist&Type=io.cozy.folders")
	assert.Equal(t, 201, res1.StatusCode)

	res2, _ := createDir(t, "/files/?Name=iexist&Type=io.cozy.folders")
	assert.Equal(t, 409, res2.StatusCode)
}

func TestCreateDirRootSuccess(t *testing.T) {
	res, _ := createDir(t, "/files/?Name=coucou&Type=io.cozy.folders")
	assert.Equal(t, 201, res.StatusCode)

	storage, _ := instance.GetStorageProvider()
	exists, err := afero.DirExists(storage, "/coucou")
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestCreateDirWithParentSuccess(t *testing.T) {
	res1, data1 := createDir(t, "/files/?Name=dirparent&Type=io.cozy.folders")
	assert.Equal(t, 201, res1.StatusCode)

	var ok bool
	data1, ok = data1["data"].(map[string]interface{})
	assert.True(t, ok)

	parentID, ok := data1["id"].(string)
	assert.True(t, ok)

	res2, _ := createDir(t, "/files/"+parentID+"?Name=child&Type=io.cozy.folders")
	assert.Equal(t, 201, res2.StatusCode)

	storage, _ := instance.GetStorageProvider()
	exists, err := afero.DirExists(storage, "/dirparent/child")
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestCreateDirWithIllegalCharacter(t *testing.T) {
	res1, _ := createDir(t, "/files/?Name=coucou/les/copains!&Type=io.cozy.folders")
	assert.Equal(t, 422, res1.StatusCode)

	res2, _ := createDir(t, "/files/?Name=j'ai\x00untrou!&Type=io.cozy.folders")
	assert.Equal(t, 422, res2.StatusCode)
}

func TestCreateDirConcurrently(t *testing.T) {
	done := make(chan *http.Response)
	errs := make(chan *http.Response)

	doCreateDir := func(name string) {
		res, _ := createDir(t, "/files/?Name="+name+"&Type=io.cozy.folders")
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
			assert.Equal(t, 409, res.StatusCode)
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
	res, _ := upload(t, "/files/?Type=io.cozy.files", "text/plain", "foo", "")
	assert.Equal(t, 422, res.StatusCode)
}

func TestUploadBadHash(t *testing.T) {
	body := "foo"
	res, _ := upload(t, "/files/?Type=io.cozy.files&Name=badhash", "text/plain", body, "3FbbMXfH+PdjAlWFfVb1dQ==")
	assert.Equal(t, 412, res.StatusCode)

	storage, _ := instance.GetStorageProvider()
	_, err := afero.ReadFile(storage, "/badhash")
	assert.Error(t, err)
}

func TestUploadAtRootSuccess(t *testing.T) {
	body := "foo"
	res, _ := upload(t, "/files/?Type=io.cozy.files&Name=goodhash", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res.StatusCode)

	storage, _ := instance.GetStorageProvider()
	buf, err := afero.ReadFile(storage, "/goodhash")
	assert.NoError(t, err)
	assert.Equal(t, body, string(buf))
}

func TestUploadConcurrently(t *testing.T) {
	done := make(chan *http.Response)
	errs := make(chan *http.Response)

	doUpload := func(name, body string) {
		res, _ := upload(t, "/files/?Type=io.cozy.files&Name="+name, "text/plain", body, "")
		if res.StatusCode == 201 {
			done <- res
		} else {
			errs <- res
		}
	}

	n := 100
	c := 0

	for i := 0; i < n; i++ {
		go doUpload("uploadedconcurrently", "body "+strconv.Itoa(i))
	}

	for i := 0; i < n; i++ {
		select {
		case res := <-errs:
			assert.Equal(t, 409, res.StatusCode)
		case <-done:
			c = c + 1
		}
	}

	assert.Equal(t, 1, c)
}

func TestUploadWithParentSuccess(t *testing.T) {
	res1, data1 := createDir(t, "/files/?Name=fileparent&Type=io.cozy.folders")
	assert.Equal(t, 201, res1.StatusCode)

	var ok bool
	data1, ok = data1["data"].(map[string]interface{})
	assert.True(t, ok)

	parentID, ok := data1["id"].(string)
	assert.True(t, ok)

	body := "foo"
	res2, _ := upload(t, "/files/"+parentID+"?Type=io.cozy.files&Name=goodhash", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res2.StatusCode)

	storage, _ := instance.GetStorageProvider()
	buf, err := afero.ReadFile(storage, "/fileparent/goodhash")
	assert.NoError(t, err)
	assert.Equal(t, body, string(buf))
}

func TestUploadAtRootAlreadyExists(t *testing.T) {
	body := "foo"
	res1, _ := upload(t, "/files/?Type=io.cozy.files&Name=iexistfile", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res1.StatusCode)

	res2, _ := upload(t, "/files/?Type=io.cozy.files&Name=iexistfile", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 409, res2.StatusCode)
}

func TestUploadWithParentAlreadyExists(t *testing.T) {
	_, dirdata := createDir(t, "/files/?Type=io.cozy.folders&Name=container")

	var ok bool
	dirdata, ok = dirdata["data"].(map[string]interface{})
	assert.True(t, ok)

	parentID, ok := dirdata["id"].(string)
	assert.True(t, ok)

	body := "foo"
	res1, _ := upload(t, "/files/"+parentID+"?Type=io.cozy.files&Name=iexistfile", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res1.StatusCode)

	res2, _ := upload(t, "/files/"+parentID+"?Type=io.cozy.files&Name=iexistfile", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 409, res2.StatusCode)
}

func TestModifyMetadataFileMove(t *testing.T) {
	body := "foo"
	res1, data1 := upload(t, "/files/?Type=io.cozy.files&Name=filemoveme&Tags=foo,bar", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res1.StatusCode)

	var ok bool
	data1, ok = data1["data"].(map[string]interface{})
	assert.True(t, ok)

	fileID, ok := data1["id"].(string)
	assert.True(t, ok)
	// fileRev, ok := data1["rev"].(string)
	// assert.True(t, ok)

	res2, data2 := createDir(t, "/files/?Name=movemeinme&Type=io.cozy.folders")
	assert.Equal(t, 201, res2.StatusCode)

	data2, ok = data2["data"].(map[string]interface{})
	assert.True(t, ok)

	folderID, ok := data2["id"].(string)
	assert.True(t, ok)

	attrs := map[string]interface{}{
		"tags":       []string{"bar", "baz"},
		"name":       "moved",
		"folder_id":  folderID,
		"executable": true,
	}

	res3, data3 := patchFile(t, "/files/"+fileID, "io.cozy.files", fileID, attrs)
	assert.Equal(t, 200, res3.StatusCode)

	data3, ok = data3["data"].(map[string]interface{})
	assert.True(t, ok)

	attrs3, ok := data3["attributes"].(map[string]interface{})
	assert.True(t, ok)

	assert.Equal(t, "text/plain", attrs3["mime"])
	assert.Equal(t, "moved", attrs3["name"])
	assert.EqualValues(t, []interface{}{"foo", "bar", "baz"}, attrs3["tags"])
	assert.Equal(t, "text", attrs3["class"])
	assert.Equal(t, "rL0Y20zC+Fzt72VPzMSk2A==", attrs3["md5sum"])
	assert.Equal(t, true, attrs3["executable"])
	assert.Equal(t, "3", attrs3["size"])
}

func TestModifyMetadataDirMove(t *testing.T) {
	res1, data1 := createDir(t, "/files/?Name=dirmodme&Type=io.cozy.folders&Tags=foo,bar,bar")
	assert.Equal(t, 201, res1.StatusCode)

	folder1ID, _ := extractDirData(t, data1)

	reschild1, _ := createDir(t, "/files/"+folder1ID+"?Name=child1&Type=io.cozy.folders")
	assert.Equal(t, 201, reschild1.StatusCode)

	reschild2, _ := createDir(t, "/files/"+folder1ID+"?Name=child2&Type=io.cozy.folders")
	assert.Equal(t, 201, reschild2.StatusCode)

	res2, data2 := createDir(t, "/files/?Name=dirmodmemoveinme&Type=io.cozy.folders")
	assert.Equal(t, 201, res2.StatusCode)

	folder2ID, _ := extractDirData(t, data2)

	attrs1 := map[string]interface{}{
		"tags":      []string{"bar", "baz"},
		"name":      "renamed",
		"folder_id": folder2ID,
	}

	res3, _ := patchFile(t, "/files/"+folder1ID, "io.cozy.folders", folder1ID, attrs1)
	assert.Equal(t, 200, res3.StatusCode)

	storage, _ := instance.GetStorageProvider()
	exists, err := afero.DirExists(storage, "/dirmodmemoveinme/renamed")
	assert.NoError(t, err)
	assert.True(t, exists)

	attrs2 := map[string]interface{}{
		"tags":      []string{"bar", "baz"},
		"name":      "renamed",
		"folder_id": folder1ID,
	}

	res4, _ := patchFile(t, "/files/"+folder2ID, "io.cozy.folders", folder2ID, attrs2)
	assert.Equal(t, 412, res4.StatusCode)

	res5, _ := patchFile(t, "/files/"+folder1ID, "io.cozy.folders", folder1ID, attrs2)
	assert.Equal(t, 412, res5.StatusCode)
}

func TestModifyContentNoFileID(t *testing.T) {
	res, _ := uploadMod(t, "/files/badid", "text/plain", "nil", "")
	assert.Equal(t, 404, res.StatusCode)
}

func TestModifyContentBadRev(t *testing.T) {
	res1, data1 := upload(t, "/files/?Type=io.cozy.files&Name=modbadrev&Executable=true", "text/plain", "foo", "")
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
	assert.NoError(t, err)

	req2.Header.Add("If-Match", "badrev")
	res2, _ := doUploadOrMod(t, req2, "text/plain", newcontent, "")
	assert.Equal(t, 412, res2.StatusCode)

	req3, err := http.NewRequest("PUT", ts.URL+"/files/"+fileID, strings.NewReader(newcontent))
	assert.NoError(t, err)

	req3.Header.Add("If-Match", fileRev)
	res3, _ := doUploadOrMod(t, req3, "text/plain", newcontent, "")
	assert.Equal(t, 200, res3.StatusCode)
}

func TestModifyContentSuccess(t *testing.T) {
	var err error
	var buf []byte
	var fileInfo os.FileInfo

	storage, _ := instance.GetStorageProvider()
	res1, data1 := upload(t, "/files/?Type=io.cozy.files&Name=willbemodified&Executable=true", "text/plain", "foo", "")
	assert.Equal(t, 201, res1.StatusCode)

	buf, err = afero.ReadFile(storage, "/willbemodified")
	assert.Equal(t, "foo", string(buf))
	fileInfo, err = storage.Stat("/willbemodified")
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

	buf, err = afero.ReadFile(storage, "/willbemodified")
	assert.NoError(t, err)
	assert.Equal(t, newcontent, string(buf))
	fileInfo, err = storage.Stat("/willbemodified")
	assert.NoError(t, err)
	assert.Equal(t, fileInfo.Mode().String(), "-rw-r--r--")
}

func TestModifyContentConcurrently(t *testing.T) {
	type result struct {
		rev string
		idx int64
	}

	done := make(chan *result)
	errs := make(chan *http.Response)

	res, data := upload(t, "/files/?Type=io.cozy.files&Name=willbemodifiedconcurrently&Executable=true", "text/plain", "foo", "")
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
			assert.True(t, res.StatusCode == 409 || res.StatusCode == 503)
		case res := <-done:
			successes = append(successes, res)
		}
	}

	assert.True(t, len(successes) >= 1)

	for i, s := range successes {
		assert.True(t, strings.HasPrefix(s.rev, strconv.Itoa(i+2)+"-"))
	}

	lastS := successes[len(successes)-1]
	storage, _ := instance.GetStorageProvider()
	buf, err := afero.ReadFile(storage, "/willbemodifiedconcurrently")
	assert.NoError(t, err)
	assert.Equal(t, "newcontent "+strconv.FormatInt(lastS.idx, 10), string(buf))
}

func TestDownloadFileBadID(t *testing.T) {
	res, _ := download(t, "/files/badid", "")
	assert.Equal(t, 404, res.StatusCode)
}

func TestDownloadFileBadPath(t *testing.T) {
	res, _ := download(t, "/files/download?Path=/i/do/not/exist", "")
	assert.Equal(t, 404, res.StatusCode)
}

func TestDownloadFileByIDSuccess(t *testing.T) {
	body := "foo"
	res1, filedata := upload(t, "/files/?Type=io.cozy.files&Name=downloadme1", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res1.StatusCode)

	var ok bool
	filedata, ok = filedata["data"].(map[string]interface{})
	assert.True(t, ok)

	fileID, ok := filedata["id"].(string)
	assert.True(t, ok)

	res2, resbody := download(t, "/files/"+fileID, "")
	assert.Equal(t, 200, res2.StatusCode)
	assert.True(t, strings.HasPrefix(res2.Header.Get("Content-Disposition"), "inline"))
	assert.True(t, strings.Contains(res2.Header.Get("Content-Disposition"), "filename=downloadme1"))
	assert.True(t, strings.HasPrefix(res2.Header.Get("Content-Type"), "text/plain"))
	assert.NotEmpty(t, res2.Header.Get("Etag"))
	assert.Equal(t, res2.Header.Get("Content-Length"), "3")
	assert.Equal(t, res2.Header.Get("Accept-Ranges"), "bytes")
	assert.Equal(t, body, string(resbody))
}

func TestDownloadFileByPathSuccess(t *testing.T) {
	body := "foo"
	res1, _ := upload(t, "/files/?Type=io.cozy.files&Name=downloadme2", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res1.StatusCode)

	res2, resbody := download(t, "/files/download?Path="+url.QueryEscape("/downloadme2"), "")
	assert.Equal(t, 200, res2.StatusCode)
	assert.True(t, strings.HasPrefix(res2.Header.Get("Content-Disposition"), "attachment"))
	assert.True(t, strings.Contains(res2.Header.Get("Content-Disposition"), "filename=downloadme2"))
	assert.True(t, strings.HasPrefix(res2.Header.Get("Content-Type"), "text/plain"))
	assert.Equal(t, res2.Header.Get("Content-Length"), "3")
	assert.Equal(t, res2.Header.Get("Accept-Ranges"), "bytes")
	assert.Equal(t, body, string(resbody))
}

func TestDownloadRangeSuccess(t *testing.T) {
	body := "foo,bar"
	res1, _ := upload(t, "/files/?Type=io.cozy.files&Name=downloadmebyrange", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
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

func TestGetFileMetadata(t *testing.T) {
	res1, _ := http.Get(ts.URL + "/files/metadata?Path=/noooooop&Type=io.cozy.files")
	assert.Equal(t, 404, res1.StatusCode)

	body := "foo,bar"
	res2, _ := upload(t, "/files/?Type=io.cozy.files&Name=getmetadata", "text/plain", body, "UmfjCVWct/albVkURcJJfg==")
	assert.Equal(t, 201, res2.StatusCode)

	res3, _ := http.Get(ts.URL + "/files/metadata?Path=/getmetadata&Type=io.cozy.files")
	assert.Equal(t, 200, res3.StatusCode)
}

func TestGetDirectoryMetadata(t *testing.T) {
	res1, data1 := createDir(t, "/files/?Name=getdirmeta&Type=io.cozy.folders")
	assert.Equal(t, 201, res1.StatusCode)

	var ok bool
	data1, ok = data1["data"].(map[string]interface{})
	assert.True(t, ok)

	parentID, ok := data1["id"].(string)
	assert.True(t, ok)

	body := "foo"
	res2, _ := upload(t, "/files/"+parentID+"?Type=io.cozy.files&Name=firstfile", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res2.StatusCode)

	res3, _ := http.Get(ts.URL + "/files/metadata?Path=/getdirmeta&Type=io.cozy.folders")
	assert.Equal(t, 200, res3.StatusCode)
}

func TestMain(m *testing.M) {
	// First we make sure couchdb is started
	db, err := checkup.HTTPChecker{URL: CouchURL}.Check()
	if err != nil || db.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
		os.Exit(1)
	}

	err = couchdb.ResetDB(TestPrefix, string(vfs.FsDocType))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = couchdb.DefineIndex(TestPrefix, vfs.FsDocType, mango.IndexOnFields("folder_id", "name"))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = couchdb.DefineIndex(TestPrefix, vfs.FsDocType, mango.IndexOnFields("path"))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = couchdb.DefineIndex(TestPrefix, vfs.FsDocType, mango.IndexOnFields("folder_id"))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	tempdir, err := ioutil.TempDir("", "cozy-stack")
	if err != nil {
		fmt.Println("Could not create temporary directory.")
		os.Exit(1)
	}
	defer func() {
		os.RemoveAll(tempdir)
	}()

	gin.SetMode(gin.TestMode)
	instance = &middlewares.Instance{
		Domain:     "test",
		StorageURL: "file://localhost" + tempdir,
	}

	router := gin.New()
	router.Use(injectInstance(instance))
	router.POST("/files/", CreationHandler)
	router.POST("/files/:folder-id", CreationHandler)
	router.PATCH("/files/:file-id", ModificationHandler)
	router.PUT("/files/:file-id", OverwriteFileContentHandler)
	router.HEAD("/files/:file-id", ReadFileContentHandler)
	router.GET("/files/:file-id", func(c *gin.Context) {
		if c.Param("file-id") == MetadataPath {
			ReadMetadataHandler(c)
		} else {
			ReadFileContentHandler(c)
		}
	})
	ts = httptest.NewServer(router)
	defer ts.Close()
	os.Exit(m.Run())
}
