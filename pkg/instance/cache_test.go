package instance

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/stretchr/testify/assert"
)

func TestGetFs(t *testing.T) {
	instance := &Instance{
		IndexViewsVersion: consts.IndexViewsVersion,
		Domain:            "test-provider.cozycloud.cc",
	}
	err := instance.makeVFS()
	if !assert.NoError(t, err) {
		return
	}
	storage := instance.VFS()
	assert.NotNil(t, storage, "the instance should have a memory storage provider")
}

func TestCache(t *testing.T) {
	globalCache = nil
	defer func() {
		globalCache = nil
	}()

	i := &Instance{
		IndexViewsVersion: consts.IndexViewsVersion,
		DocID:             "fake-instance",
		Domain:            "cached.cozy.tools",
		Locale:            "zh",
	}
	getCache().Set("cached.cozy.tools", i)

	i2, err := Get("cached.cozy.tools")
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, i2.Locale, "zh")

	globalCache.Revoke("cached.cozy.tools")

	_, err = Get("cached.cozy.tools")
	assert.Error(t, err)

}
