package middlewares_test

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/stretchr/testify/assert"
)

func TestSplitHost(t *testing.T) {
	config.UseTestFile()
	cfg := config.GetConfig()
	was := cfg.Subdomains
	defer func() { cfg.Subdomains = was }()

	host, app, siblings := middlewares.SplitHost("localhost")
	assert.Equal(t, "localhost", host)
	assert.Equal(t, "", app)
	assert.Equal(t, "", siblings)

	cfg.Subdomains = config.NestedSubdomains
	host, app, siblings = middlewares.SplitHost("calendar.joe.example.net")
	assert.Equal(t, "joe.example.net", host)
	assert.Equal(t, "calendar", app)
	assert.Equal(t, "*.joe.example.net", siblings)

	cfg.Subdomains = config.FlatSubdomains
	host, app, siblings = middlewares.SplitHost("joe-calendar.example.net")
	assert.Equal(t, "joe.example.net", host)
	assert.Equal(t, "calendar", app)
	assert.Equal(t, "*.example.net", siblings)

	host, app, siblings = middlewares.SplitHost("joe.example.net")
	assert.Equal(t, "joe.example.net", host)
	assert.Equal(t, "", app)
	assert.Equal(t, "", siblings)
}
