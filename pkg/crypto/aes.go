package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
)

// EncryptWithAES256CBC uses AES-256-CBC to encrypt the payload, and returns a
// bitwarden cipher string.
// See https://github.com/jcs/rubywarden/blob/master/API.md#example
func EncryptWithAES256CBC(key, payload, iv []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	dst := make([]byte, len(payload)+aes.BlockSize)
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(dst[:len(payload)], payload)
	padding := make([]byte, aes.BlockSize)
	for i := range padding {
		padding[i] = aes.BlockSize
	}
	mode.CryptBlocks(dst[len(payload):], padding)
	iv64 := base64.StdEncoding.EncodeToString(iv)
	dst64 := base64.StdEncoding.EncodeToString(dst)

	// 0 means AES-256-CBC
	cipherString := "0." + iv64 + "|" + dst64
	return cipherString, nil
}
