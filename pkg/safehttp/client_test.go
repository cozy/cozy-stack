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

func TestSetTrustedPrivateNetworks(t *testing.T) {
	t.Run("invalid CIDR returns error", func(t *testing.T) {
		err := SetTrustedPrivateNetworks([]string{"not-a-cidr"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid trusted private network CIDR")
	})

	t.Run("valid CIDRs are accepted", func(t *testing.T) {
		err := SetTrustedPrivateNetworks([]string{"10.0.0.0/8", "192.168.0.0/16"})
		assert.NoError(t, err)
		t.Cleanup(resetTrustedNetworks)
	})

	t.Run("invalid CIDR does not change existing allowlist", func(t *testing.T) {
		err := SetTrustedPrivateNetworks([]string{"10.0.0.0/8"})
		require.NoError(t, err)
		require.Len(t, loadTrustedNetworks(), 1)

		err = SetTrustedPrivateNetworks([]string{"bad"})
		require.Error(t, err)
		// The previous valid config is still in place.
		assert.Len(t, loadTrustedNetworks(), 1)
		t.Cleanup(resetTrustedNetworks)
	})

	t.Run("empty list clears allowlist", func(t *testing.T) {
		err := SetTrustedPrivateNetworks([]string{"10.0.0.0/8"})
		require.NoError(t, err)

		err = SetTrustedPrivateNetworks([]string{})
		require.NoError(t, err)
		assert.Empty(t, loadTrustedNetworks())
	})
}

func resetTrustedNetworks() {
	_ = SetTrustedPrivateNetworks(nil)
}

func TestSafeControl(t *testing.T) {
	build.BuildMode = build.ModeProd
	t.Cleanup(resetTrustedNetworks)

	t.Run("private IP blocked by default", func(t *testing.T) {
		err := safeControl("tcp4", "192.168.1.1:80", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "is not a public IP address")
	})

	t.Run("loopback blocked by default", func(t *testing.T) {
		err := safeControl("tcp4", "127.0.0.1:80", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "is not a public IP address")
	})

	t.Run("private IP allowed when inside configured CIDR", func(t *testing.T) {
		require.NoError(t, SetTrustedPrivateNetworks([]string{"192.168.0.0/16"}))
		err := safeControl("tcp4", "192.168.1.1:80", nil)
		assert.NoError(t, err)
	})

	t.Run("private IP on non-standard port allowed when inside configured CIDR", func(t *testing.T) {
		require.NoError(t, SetTrustedPrivateNetworks([]string{"10.0.0.0/8"}))
		err := safeControl("tcp4", "10.0.0.1:8080", nil)
		assert.NoError(t, err)
	})

	t.Run("private IP outside configured CIDR remains blocked", func(t *testing.T) {
		require.NoError(t, SetTrustedPrivateNetworks([]string{"10.0.0.0/8"}))
		err := safeControl("tcp4", "192.168.1.1:80", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "is not a public IP address")
	})

	t.Run("loopback allowed only if explicitly configured", func(t *testing.T) {
		require.NoError(t, SetTrustedPrivateNetworks([]string{"192.168.0.0/16"}))
		err := safeControl("tcp4", "127.0.0.1:80", nil)
		assert.Error(t, err)

		require.NoError(t, SetTrustedPrivateNetworks([]string{"127.0.0.0/8"}))
		err = safeControl("tcp4", "127.0.0.1:80", nil)
		assert.NoError(t, err)
	})

	t.Run("link-local remains blocked even with trusted networks", func(t *testing.T) {
		require.NoError(t, SetTrustedPrivateNetworks([]string{"0.0.0.0/0"}))
		err := safeControl("tcp4", "169.254.1.1:80", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "is not a valid IP address")
	})

	t.Run("unspecified address remains blocked even with trusted networks", func(t *testing.T) {
		require.NoError(t, SetTrustedPrivateNetworks([]string{"0.0.0.0/0"}))
		err := safeControl("tcp4", "0.0.0.0:80", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "is not a valid IP address")
	})

	t.Run("unsupported network type rejected", func(t *testing.T) {
		err := safeControl("udp", "10.0.0.1:80", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "is not a safe network type")
	})

	t.Run("public IP on non-standard port still blocked", func(t *testing.T) {
		require.NoError(t, SetTrustedPrivateNetworks([]string{"10.0.0.0/8"}))
		err := safeControl("tcp4", "8.8.8.8:8080", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "is not a safe port number")
	})

	t.Run("public IP on non-standard port blocked even with broad trusted CIDR", func(t *testing.T) {
		require.NoError(t, SetTrustedPrivateNetworks([]string{"0.0.0.0/0"}))
		err := safeControl("tcp4", "8.8.8.8:8080", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "is not a safe port number")
	})

	t.Run("IPv6 private address with matching CIDR", func(t *testing.T) {
		require.NoError(t, SetTrustedPrivateNetworks([]string{"::1/128"}))
		err := safeControl("tcp6", "[::1]:8080", nil)
		assert.NoError(t, err)
	})
}
