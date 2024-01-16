package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
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
	if _, err := hash.Write(iv); err != nil {
		return "", err
	}
	if _, err := hash.Write(dst); err != nil {
		return "", err
	}
	h64 := base64.StdEncoding.EncodeToString(hash.Sum(nil))

	// 2 means AesCbc256_HmacSha256_B64
	cipherString := "2." + iv64 + "|" + dst64 + "|" + h64
	return cipherString, nil
}

// UnwrapA256KW decrypts the provided cipher text with the given AES cipher
// (and corresponding key), using the AES Key Wrap algorithm (RFC-3394). The
// decrypted cipher text is verified using the default IV and will return an
// error if validation fails.
//
// Taken from https://github.com/NickBall/go-aes-key-wrap/blob/master/keywrap.go
func UnwrapA256KW(block cipher.Block, cipherText []byte) ([]byte, error) {
	a := make([]byte, 8)
	n := (len(cipherText) / 8) - 1

	r := make([][]byte, n)
	for i := range r {
		r[i] = make([]byte, 8)
		copy(r[i], cipherText[(i+1)*8:])
	}
	copy(a, cipherText[:8])

	for j := 5; j >= 0; j-- {
		for i := n; i >= 1; i-- {
			t := (n * j) + i
			tBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(tBytes, uint64(t))

			b := arrConcat(arrXor(a, tBytes), r[i-1])
			block.Decrypt(b, b)

			copy(a, b[:len(b)/2])
			copy(r[i-1], b[len(b)/2:])
		}
	}

	if subtle.ConstantTimeCompare(a, defaultIV) != 1 {
		return nil, errors.New("integrity check failed - unexpected IV")
	}

	c := arrConcat(r...)
	return c, nil
}

// defaultIV as specified in RFC-3394
var defaultIV = []byte{0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6, 0xA6}

func arrConcat(arrays ...[]byte) []byte {
	out := make([]byte, len(arrays[0]))
	copy(out, arrays[0])
	for _, array := range arrays[1:] {
		out = append(out, array...)
	}
	return out
}

func arrXor(arrL []byte, arrR []byte) []byte {
	out := make([]byte, len(arrL))
	for x := range arrL {
		out[x] = arrL[x] ^ arrR[x]
	}
	return out
}
