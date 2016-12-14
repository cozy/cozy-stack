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

func TestEncoding(t *testing.T) {
	for _, value := range testStrings {
		encoded := Base64Encode([]byte(value))
		decoded, err := Base64Decode(encoded)
		if !assert.NoError(t, err) {
			return
		}
		if !assert.Equal(t, value, string(decoded)) {
			return
		}
	}
}
