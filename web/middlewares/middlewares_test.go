package middlewares

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestSplitHost(t *testing.T) {
	config.UseTestFile()
	cfg := config.GetConfig()
	was := cfg.Subdomains
	defer func() { cfg.Subdomains = was }()

	host, app := SplitHost("localhost")
	assert.Equal(t, "localhost", host)
	assert.Equal(t, "", app)

	cfg.Subdomains = config.NestedSubdomains
	host, app = SplitHost("calendar.joe.example.net")
	assert.Equal(t, "joe.example.net", host)
	assert.Equal(t, "calendar", app)

	cfg.Subdomains = config.FlatSubdomains
	host, app = SplitHost("joe-calendar.example.net")
	assert.Equal(t, "joe.example.net", host)
	assert.Equal(t, "calendar", app)

	host, app = SplitHost("joe.example.net")
	assert.Equal(t, "joe.example.net", host)
	assert.Equal(t, "", app)
}
