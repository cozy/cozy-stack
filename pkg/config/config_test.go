package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestUseViper(t *testing.T) {
	cfg := viper.New()
	cfg.Set("couchdb.host", "db")
	cfg.Set("couchdb.port", 1234)

	UseViper(cfg)

	assert.Equal(t, "http://db:1234/", CouchURL())
}
