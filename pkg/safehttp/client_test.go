package safehttp

import (
	"testing"

	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultClient(t *testing.T) {
	build.BuildMode = build.ModeProd

	res, err := DefaultClient.Get("https://github.com/")
	require.NoError(t, err)
	defer res.Body.Close()

	_, err = DefaultClient.Get("http://192.168.0.1/")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a public IP address")

	_, err = DefaultClient.Get("http://1.2.3.4:5984/")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a safe port")
}

func TestSetAllowedPrivateNetworks(t *testing.T) {
	build.BuildMode = build.ModeProd
	t.Cleanup(func() {
		allowedPrivateNets = nil
	})

	// Invalid CIDR should return an error.
	err := SetAllowedPrivateNetworks([]string{"not-a-cidr"})
	require.Error(t, err)

	// Private address blocked without allowlist.
	allowedPrivateNets = nil
	err = safeControl("tcp4", "10.0.0.1:443", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a public IP address")

	// Private address allowed when its network is in the allowlist.
	require.NoError(t, SetAllowedPrivateNetworks([]string{"10.0.0.0/8"}))
	err = safeControl("tcp4", "10.0.0.1:443", nil)
	assert.NoError(t, err)

	// An address outside the allowlist is still rejected.
	err = safeControl("tcp4", "192.168.1.1:443", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a public IP address")
}
