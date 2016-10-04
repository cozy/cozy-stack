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

func upload(t *testing.T, path, contentType, body string) *http.Response {
	buf := strings.NewReader(body)
	res, err := http.Post(ts.URL+path, contentType, buf)
	assert.NoError(t, err)
	return res
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
	buf, err := afero.ReadFile(*storage, "123/bar")
	assert.NoError(t, err)
	assert.Equal(t, body, string(buf))
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	instance = &middlewares.Instance{
		Domain:     "test",
		StorageURL: "mem://test",
	}
	router := gin.New()
	router.Use(injectInstance(instance))
	router.POST("/files/:folder-id", Upload)
	ts = httptest.NewServer(router)
	defer ts.Close()
	os.Exit(m.Run())
}
