package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
)

func addPadding(payload []byte) []byte {
	l := len(payload)
	p := aes.BlockSize - (l % aes.BlockSize)
	padded := make([]byte, l+p)
	copy(padded, payload)
	for i := 0; i < p; i++ {
		padded[l+i] = byte(p)
	}
	return padded
}

func encryptAES256(key, payload, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	dst := make([]byte, len(payload))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(dst, payload)
	return dst, nil
}

// EncryptWithAES256CBC uses AES-256-CBC to encrypt the payload, and returns a
// bitwarden cipher string.
// See https://github.com/jcs/rubywarden/blob/master/API.md#example
func EncryptWithAES256CBC(key, payload, iv []byte) (string, error) {
	payload = addPadding(payload)
	dst, err := encryptAES256(key, payload, iv)
	if err != nil {
		return "", err
	}
	iv64 := base64.StdEncoding.EncodeToString(iv)
	dst64 := base64.StdEncoding.EncodeToString(dst)

	// 0 means AesCbc256_B64
	cipherString := "0." + iv64 + "|" + dst64
	return cipherString, nil
}

// EncryptWithAES256HMAC uses AES-256-CBC with HMAC SHA-256 to encrypt the
// payload, and returns a bitwarden cipher string.
func EncryptWithAES256HMAC(encKey, macKey, payload, iv []byte) (string, error) {
	payload = addPadding(payload)
	dst, err := encryptAES256(encKey, payload, iv)
	if err != nil {
		return "", err
	}
	iv64 := base64.StdEncoding.EncodeToString(iv)
	dst64 := base64.StdEncoding.EncodeToString(dst)

	hash := hmac.New(sha256.New, macKey)
	hash.Write(iv)
	hash.Write(dst)
	h64 := base64.StdEncoding.EncodeToString(hash.Sum(nil))

	// 2 means AesCbc256_HmacSha256_B64
	cipherString := "2." + iv64 + "|" + dst64 + "|" + h64
	return cipherString, nil
}
