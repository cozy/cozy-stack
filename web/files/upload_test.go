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

func TestUploadWithNoType(t *testing.T) {
	buf := strings.NewReader("foo")
	res, err := http.Post(ts.URL+"/files/123", "text/plain", buf)
	assert.NoError(t, err)
	assert.Equal(t, 422, res.StatusCode)
	res.Body.Close()
}

func TestUploadWithNoName(t *testing.T) {
	buf := strings.NewReader("foo")
	res, err := http.Post(ts.URL+"/files/123?Type=io.cozy.files", "text/plain", buf)
	assert.NoError(t, err)
	assert.Equal(t, 422, res.StatusCode)
	res.Body.Close()
}

func TestUploadSuccess(t *testing.T) {
	foo := []byte{'f', 'o', 'o'}
	body := strings.NewReader(string(foo))
	res, err := http.Post(ts.URL+"/files/123?Type=io.cozy.files&Name=bar", "text/plain", body)
	assert.NoError(t, err)
	assert.Equal(t, 201, res.StatusCode)
	res.Body.Close()
	storage, _ := instance.GetStorageProvider()
	buf, err := afero.ReadFile(*storage, "123/bar")
	assert.NoError(t, err)
	assert.Equal(t, foo, buf)
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
