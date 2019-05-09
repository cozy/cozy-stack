package app

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitChannelVersionRegistryFetcher(t *testing.T) {
	url, err := url.Parse("registry://freemobile/stable/latest")
	c, v := getRegistryChannel(url)
	assert.NoError(t, err)
	assert.EqualValues(t, c, "stable")
	assert.EqualValues(t, v, "")

	url, err2 := url.Parse("registry://freemobile/stable/1.2.0")
	c2, v2 := getRegistryChannel(url)
	assert.NoError(t, err2)
	assert.EqualValues(t, c2, "stable")
	assert.EqualValues(t, v2, "1.2.0")

	url, err3 := url.Parse("registry://freemobile/1.0.2")
	c3, v3 := getRegistryChannel(url)
	assert.NoError(t, err3)
	assert.EqualValues(t, c3, "stable")
	assert.EqualValues(t, v3, "1.0.2")

	url, err4 := url.Parse("registry://freemobile/beta")
	c4, v4 := getRegistryChannel(url)
	assert.NoError(t, err4)
	assert.EqualValues(t, c4, "beta")
	assert.EqualValues(t, v4, "")

	url, err5 := url.Parse("registry://freemobile")
	c5, v5 := getRegistryChannel(url)
	assert.NoError(t, err5)
	assert.EqualValues(t, c5, "stable")
	assert.EqualValues(t, v5, "")

}
