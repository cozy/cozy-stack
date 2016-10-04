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

func upload(t *testing.T, path, contentType, body string) *http.Response {
	buf := strings.NewReader(body)
	res, err := http.Post(ts.URL+path, contentType, buf)
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

func TestUploadWithNoType(t *testing.T) {
	res := upload(t, "/files/123", "text/plain", "foo")
	assert.Equal(t, 422, res.StatusCode)
	res.Body.Close()
}

func TestUploadWithNoName(t *testing.T) {
	res := upload(t, "/files/123?Type=io.cozy.files", "text/plain", "foo")
	assert.Equal(t, 422, res.StatusCode)
	res.Body.Close()
}

func TestUploadSuccess(t *testing.T) {
	body := "foo"
	res := upload(t, "/files/123?Type=io.cozy.files&Name=bar", "text/plain", body)
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
	storage.Mkdir("/123", 0777)

	router := gin.New()
	router.Use(injectInstance(instance))
	router.POST("/files/:folder-id", FolderPostHandler)
	ts = httptest.NewServer(router)
	defer ts.Close()
	os.Exit(m.Run())
}
