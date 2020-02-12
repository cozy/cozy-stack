package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitHost(t *testing.T) {
	UseTestFile()
	cfg := GetConfig()
	was := cfg.Subdomains
	defer func() { cfg.Subdomains = was }()

	host, app, siblings := SplitCozyHost("localhost")
	assert.Equal(t, "localhost", host)
	assert.Equal(t, "", app)
	assert.Equal(t, "", siblings)

	cfg.Subdomains = NestedSubdomains
	host, app, siblings = SplitCozyHost("calendar.joe.example.net")
	assert.Equal(t, "joe.example.net", host)
	assert.Equal(t, "calendar", app)
	assert.Equal(t, "*.joe.example.net", siblings)

	cfg.Subdomains = FlatSubdomains
	host, app, siblings = SplitCozyHost("joe-calendar.example.net")
	assert.Equal(t, "joe.example.net", host)
	assert.Equal(t, "calendar", app)
	assert.Equal(t, "*.example.net", siblings)

	host, app, siblings = SplitCozyHost("joe.example.net")
	assert.Equal(t, "joe.example.net", host)
	assert.Equal(t, "", app)
	assert.Equal(t, "", siblings)
}
