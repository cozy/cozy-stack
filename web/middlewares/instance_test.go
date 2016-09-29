package middlewares

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

func TestGetStorageProvider(t *testing.T) {
	instance := Instance{
		Domain:     "test.cozycloud.cc",
		StorageURL: "mem://test",
	}
	content := []byte{'b', 'a', 'r'}
	storage, err := instance.GetStorageProvider()
	assert.NoError(t, err)
	assert.NotNil(t, storage, "the instance should have a memory storage provider")
	err = afero.WriteFile(storage, "foo", content, 0644)
	assert.NoError(t, err)
	storage, err = instance.GetStorageProvider()
	assert.NoError(t, err)
	assert.NotNil(t, storage, "the instance should have a memory storage provider")
	buf, err := afero.ReadFile(storage, "foo")
	assert.NoError(t, err)
	assert.Equal(t, content, buf, "the storage should have persist the content of the foo file")
}

func TestSetInstance(t *testing.T) {
	router := gin.New()
	router.Use(SetInstance())
	router.GET("/", func(c *gin.Context) {
		instanceInterface, exists := c.Get("instance")
		assert.True(t, exists, "the instance should have been set in the gin context")
		instance := instanceInterface.(Instance)
		assert.Equal(t, "dev", instance.Domain, "the domain should have been set in the instance")
		storage, err := instance.GetStorageProvider()
		assert.NoError(t, err)
		assert.NotNil(t, storage, "the instance should have a storage provider")
		c.String(http.StatusOK, "OK")
	})
	ts := httptest.NewServer(router)
	defer ts.Close()
	res, err := http.Get(ts.URL + "/")
	assert.NoError(t, err)
	res.Body.Close()
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}
