package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func makeBuf(l int) []byte {
	buf := make([]byte, l)
	for i := range buf {
		buf[i] = 'x'
	}
	return buf
}

func TestEncryptWithAES256CBC(t *testing.T) {
	key := makeBuf(32)
	payload := makeBuf(64)
	iv := makeBuf(16)
	str, err := EncryptWithAES256CBC(key, payload, iv)
	assert.NoError(t, err)
	expected := "0.eHh4eHh4eHh4eHh4eHh4eA==|xYfs1rg9exsctltfkrSi7gROm5RjRhnF71zYvz3zHeIn6UIIoZP3Whbh6CSbbdfd0sY8kA2qs2Fziv+9bk4tATrLaSs3RqDUcdnZvFl2l7E="
	assert.Equal(t, expected, str)
	// In ruby:
	// require 'base64'
	// require 'openssl'
	// key = 'x' * 32
	// pt = 'x' * 64
	// iv = 'x' * 16
	// cipher = OpenSSL::Cipher.new "AES-256-CBC"
	// cipher.encrypt
	// cipher.key = key
	// cipher.iv = iv
	// ct = cipher.update(pt)
	// ct << cipher.final
	// expected = "0." + Base64.strict_encode64(iv) + "|" + Base64.strict_encode64(ct)
}
