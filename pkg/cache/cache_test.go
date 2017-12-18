package cache

import (
	"os"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/stretchr/testify/assert"
)

type testStruct struct {
	Test string
}

func TestGetClient(t *testing.T) {
	cache := Create("testns", time.Minute)

	var res testStruct
	cache.Get("some-key", &res)
	assert.Empty(t, res.Test)

	cache.Set("some-key", &testStruct{"a-value"})
	var res2 testStruct
	cache.Get("some-key", &res2)
	assert.Equal(t, "a-value", res2.Test)

	cache.Del("some-key")
	var res3 testStruct
	assert.Empty(t, res3.Test)
}

func TestGetClientNoRedis(t *testing.T) {
	backConfig := config.GetConfig().Cache
	var err error
	config.GetConfig().Cache, err = config.NewRedisConfig("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { config.GetConfig().Cache = backConfig }()

	cache := Create("testns", time.Minute)
	assert.NotNil(t, cache, "we should get a cache if there is no redis url")

	var value testStruct
	assert.NotPanics(t, func() {
		cache.Get("some-key", &value)
		cache.Set("some-key", &testStruct{"a-value"})
		cache.Del("some-key")
	})
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	os.Exit(m.Run())
}
