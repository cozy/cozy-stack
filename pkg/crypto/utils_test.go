package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateRandomBytes(t *testing.T) {
	val := GenerateRandomBytes(16)
	assert.Len(t, val, 16)
	assert.NotEmpty(t, string(val))
}

func TestEncoding(t *testing.T) {
	for _, value := range testStrings {
		encoded := Base64Encode([]byte(value))
		decoded, err := Base64Decode(encoded)
		require.NoError(t, err)

		if !assert.Equal(t, value, string(decoded)) {
			return
		}
	}
}
