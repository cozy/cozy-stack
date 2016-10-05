package files

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gin-gonic/gin"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

var ts *httptest.Server
var instance *middlewares.Instance

func injectInstance(instance *middlewares.Instance) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("instance", instance)
	}
}

func createDir(t *testing.T, path string) *http.Response {
	res, err := http.Post(ts.URL+path, "text/plain", strings.NewReader(""))
	assert.NoError(t, err)
	return res
}

func upload(t *testing.T, path, contentType, body, hash string) *http.Response {
	buf := strings.NewReader(body)
	req, err := http.NewRequest("POST", ts.URL+path, buf)
	assert.NoError(t, err)

	req.Header.Add("Content-MD5", hash)

	res, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	return res
}

func TestCreateDirWithNoType(t *testing.T) {
	res := createDir(t, "/files/123")
	assert.Equal(t, 422, res.StatusCode)
	res.Body.Close()
}

func TestCreateDirWithNoName(t *testing.T) {
	res := createDir(t, "/files/123?Type=io.cozy.folders")
	assert.Equal(t, 422, res.StatusCode)
	res.Body.Close()
}

func TestCreateDirOnNonExistingParent(t *testing.T) {
	res := createDir(t, "/files/noooop?Name=foo&Type=io.cozy.folders")
	assert.Equal(t, 404, res.StatusCode)
	res.Body.Close()
}

func TestCreateDirAlreadyExists(t *testing.T) {
	res := createDir(t, "/files/123?Name=456&Type=io.cozy.folders")
	assert.Equal(t, 409, res.StatusCode)
	res.Body.Close()
}

func TestCreateDirSuccess(t *testing.T) {
	res := createDir(t, "/files/123?Name=coucou&Type=io.cozy.folders")
	assert.Equal(t, 201, res.StatusCode)
	res.Body.Close()

	storage, _ := instance.GetStorageProvider()
	exists, err := afero.DirExists(storage, "123/coucou")
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestCreateDirWithIllegalCharacter(t *testing.T) {
	res := createDir(t, "/files/123?Name=coucou/les/copains!&Type=io.cozy.folders")
	assert.Equal(t, 422, res.StatusCode)
	res.Body.Close()

	res = createDir(t, "/files/123?Name=j'ai\x00untrou!&Type=io.cozy.folders")
	assert.Equal(t, 422, res.StatusCode)
	res.Body.Close()
}

func TestUploadWithNoType(t *testing.T) {
	res := upload(t, "/files/123", "text/plain", "foo", "")
	assert.Equal(t, 422, res.StatusCode)
	res.Body.Close()
}

func TestUploadWithNoName(t *testing.T) {
	res := upload(t, "/files/123?Type=io.cozy.files", "text/plain", "foo", "")
	assert.Equal(t, 422, res.StatusCode)
	res.Body.Close()
}

func TestUploadBadHash(t *testing.T) {
	body := "foo"
	res := upload(t, "/files/123?Type=io.cozy.files&Name=quz", "text/plain", body, "3FbbMXfH+PdjAlWFfVb1dQ==")
	assert.Equal(t, 412, res.StatusCode)
	res.Body.Close()
}

func TestUploadSuccess(t *testing.T) {
	body := "foo"
	res := upload(t, "/files/123?Type=io.cozy.files&Name=bar", "text/plain", body, "rL0Y20zC+Fzt72VPzMSk2A==")
	assert.Equal(t, 201, res.StatusCode)
	res.Body.Close()

	storage, _ := instance.GetStorageProvider()
	buf, err := afero.ReadFile(storage, "123/bar")
	assert.NoError(t, err)
	assert.Equal(t, body, string(buf))
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	instance = &middlewares.Instance{
		Domain:     "test",
		StorageURL: "mem://test",
	}

	storage, _ := instance.GetStorageProvider()
	storage.Mkdir("123", 0777)
	storage.Mkdir("123/456", 0777)

	router := gin.New()
	router.Use(injectInstance(instance))
	router.POST("/files/:folder-id", CreationHandler)
	ts = httptest.NewServer(router)
	defer ts.Close()
	os.Exit(m.Run())
}
