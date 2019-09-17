package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"errors"
)

// GenerateRSAKeyPair generates a key pair that can be used for RSA. The
// private key is exported as PKCS#8, and the public key is exported as PKIX,
// and then encoded in base64.
func GenerateRSAKeyPair() (string, []byte, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", nil, err
	}
	privExported, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return "", nil, err
	}

	pubKey, err := x509.MarshalPKIXPublicKey(privKey.Public())
	if err != nil {
		return "", nil, err
	}
	pub64 := base64.StdEncoding.EncodeToString(pubKey)
	return pub64, privExported, nil
}

// EncryptWithRSA uses RSA-2048-OAEP-SHA1 to encrypt the payload, and returns a
// bitwarden cipher string.
func EncryptWithRSA(key string, payload []byte) (string, error) {
	src, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return "", nil
	}
	pubKey, err := x509.ParsePKIXPublicKey(src)
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

	// 4 means Rsa2048_OaepSha1_B64
	cipherString := "4." + dst64
	return cipherString, nil
}
