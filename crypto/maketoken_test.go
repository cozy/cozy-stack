package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateRandomBytes(t *testing.T) {
	val, err := GenerateRandomBytes(16)
	assert.NoError(t, err)
	assert.Len(t, val, 16)
	assert.NotEmpty(t, string(val))
}
