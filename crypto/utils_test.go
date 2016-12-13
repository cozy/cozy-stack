package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateRandomBytes(t *testing.T) {
	val := GenerateRandomBytes(16)
	assert.Len(t, val, 16)
	assert.NotEmpty(t, string(val))
}
