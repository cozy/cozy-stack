package safehttp

import (
	"testing"

	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultClient(t *testing.T) {
	build.BuildMode = build.ModeProd

	res, err := DefaultClient.Get("https://github.com/")
	require.NoError(t, err)
	defer res.Body.Close()

	//nolint: bodyclose // This request fail so there is no body to close
	_, err = DefaultClient.Get("http://192.168.0.1/")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a public IP address")

	//nolint: bodyclose // This request fail so there is no body to close
	_, err = DefaultClient.Get("http://1.2.3.4:5984/")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a safe port")

	testutils.TODO(t, "2023-03-01", "Replace isPrivateIP by net#IP.IsPrivate")
}
