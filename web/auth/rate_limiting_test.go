package auth_test

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/stretchr/testify/assert"
)

func TestLoginRateNotExceededMem(t *testing.T) {
	auth.GlobalCounter = auth.NewMemCounter()
	assert.NoError(t, auth.CheckRateLimit(testInstance, "auth"))

}
func TestLoginRateExceededMem(t *testing.T) {
	auth.GlobalCounter = auth.NewMemCounter()
	for i := 1; i <= 1000; i++ {
		assert.NoError(t, auth.CheckRateLimit(testInstance, "auth"))
	}

	err := auth.CheckRateLimit(testInstance, "auth")
	assert.Error(t, err)
}

func TestLoginRateNotExceededRedis(t *testing.T) {
	client := config.GetConfig().DownloadStorage.Client()
	client.Del("auth:" + testInstance.Domain)
	auth.GlobalCounter = &auth.RedisCounter{Client: client}

	assert.NoError(t, auth.CheckRateLimit(testInstance, "auth"))
}
func TestLoginRateExceededRedis(t *testing.T) {
	client := config.GetConfig().DownloadStorage.Client()
	auth.GlobalCounter = &auth.RedisCounter{Client: client}
	client.Del("auth:" + testInstance.Domain)

	for i := 1; i <= 1000; i++ {
		assert.NoError(t, auth.CheckRateLimit(testInstance, "auth"))
	}
	assert.Error(t, auth.CheckRateLimit(testInstance, "auth"))

}
