package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateTokenExchangeScope(t *testing.T) {
	assert.Error(t, validateTokenExchangeScope(""))
	assert.Error(t, validateTokenExchangeScope("io.cozy.unknown"))
	assert.Error(t, validateTokenExchangeScope("io.cozy.files\tio.cozy.contacts\tio.cozy.contacts.groups"))
	assert.NoError(t, validateTokenExchangeScope("io.cozy.files"))
	assert.NoError(t, validateTokenExchangeScope("io.cozy.files io.cozy.contacts io.cozy.contacts.groups"))
}
