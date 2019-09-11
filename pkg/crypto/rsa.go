package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"errors"
)

// EncryptWithRSA uses RSA-2048-OAEP-SHA1 to encrypt the payload, and returns a
// bitwarden cipher string.
func EncryptWithRSA(key string, payload []byte) (string, error) {
	src, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return "", nil
	}
	pubKey, err := x509.ParsePKIXPublicKey([]byte(src))
	if err != nil {
		return "", err
	}
	pub, ok := pubKey.(*rsa.PublicKey)
	if !ok {
		return "", errors.New("Invalid public key")
	}

	hash := sha1.New()
	rng := rand.Reader
	dst, err := rsa.EncryptOAEP(hash, rng, pub, payload, nil)
	if err != nil {
		return "", err
	}
	dst64 := base64.StdEncoding.EncodeToString(dst)

	// 4 means RSA-2048-OAEP-SHA1
	cipherString := "4." + dst64
	return cipherString, nil
}
