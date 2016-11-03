package config

import (
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestUseViper(t *testing.T) {
	cfg := viper.New()
	cfg.Set("mode", "production")
	cfg.Set("database.url", "http://db:42")

	UseViper(cfg)

	assert.Equal(t, Production, GetConfig().Mode)
	assert.Equal(t, "http://db:42", GetConfig().Database.URL)
}
