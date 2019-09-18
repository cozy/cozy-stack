package bitwarden

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateDomain(t *testing.T) {
	err := validateDomain("")
	assert.Equal(t, err.Error(), "Unauthorized domain")

	err = validateDomain("foo bar baz")
	assert.Equal(t, err.Error(), "Invalid domain")

	err = validateDomain("192.168.0.1")
	assert.Equal(t, err.Error(), "IP address are not authorized")

	err = validateDomain("example.com")
	assert.NoError(t, err)
}

func TestDownloadIcon(t *testing.T) {
	icon, err := downloadFavicon("github.com")
	assert.NoError(t, err)
	assert.Equal(t, icon.Mime, "image/x-icon")
}
