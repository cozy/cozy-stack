package files

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
	"github.com/sourcegraph/checkup"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

const CouchURL = "http://localhost:5984/"

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

	fileID, ok := data1["id"].(string)
	assert.True(t, ok)
	fileRev, ok := data1["rev"].(string)
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

	fileID, ok := data1["id"].(string)
	assert.True(t, ok)

	newcontent := "newcontent :)"
	res2, data2 := uploadMod(t, "/files/"+fileID+"?Executable=false", "audio/mp3", newcontent, "")
	assert.Equal(t, 200, res2.StatusCode)

	data2, ok = data2["data"].(map[string]interface{})
	assert.True(t, ok)

	attrs2, ok := data2["attributes"].(map[string]interface{})
	assert.True(t, ok)

	assert.Equal(t, data2["id"], data1["id"], "same id")
	assert.Equal(t, data2["path"], data1["path"], "same path")
	assert.NotEqual(t, data2["rev"], data1["res"], "different rev")

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

func TestDownloadFileBadID(t *testing.T) {
	res, _ := download(t, "/files/badid", "")
	assert.Equal(t, 404, res.StatusCode)
}

func TestDownloadFileBadPath(t *testing.T) {
	res, _ := download(t, "/files/download?path=/i/do/not/exist", "")
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

	res2, resbody := download(t, "/files/download?path="+url.QueryEscape("/downloadme2"), "")
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

	res2, _ := download(t, "/files/download?path="+url.QueryEscape("/downloadmebyrange"), "nimp")
	assert.Equal(t, 416, res2.StatusCode)

	res3, res3body := download(t, "/files/download?path="+url.QueryEscape("/downloadmebyrange"), "bytes=0-2")
	assert.Equal(t, 206, res3.StatusCode)
	assert.Equal(t, "foo", string(res3body))

	res4, res4body := download(t, "/files/download?path="+url.QueryEscape("/downloadmebyrange"), "bytes=4-")
	assert.Equal(t, 206, res4.StatusCode)
	assert.Equal(t, "bar", string(res4body))
}

func TestMain(m *testing.M) {
	// First we make sure couchdb is started
	couchdb, err := checkup.HTTPChecker{URL: CouchURL}.Check()
	if err != nil || couchdb.Status() != checkup.Healthy {
		fmt.Println("This test need couchdb to run.")
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
	router.PUT("/files/:file-id", OverwriteFileContentHandler)
	router.HEAD("/files/:file-id", ReadFileHandler)
	router.GET("/files/:file-id", ReadFileHandler)
	ts = httptest.NewServer(router)
	defer ts.Close()
	os.Exit(m.Run())
}
