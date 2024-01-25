package oauth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/logger"
	jwt "github.com/golang-jwt/jwt/v5"
)

// checkPlayIntegrityAttestation will check an attestation made by the Play
// Integrity API.
// https://developer.android.com/google/play/integrity
func (c *Client) checkPlayIntegrityAttestation(inst *instance.Instance, req AttestationRequest) error {
	store := GetStore()
	if ok := store.CheckAndClearChallenge(inst, c.ID(), req.Challenge); !ok {
		return errors.New("invalid challenge")
	}

	token, err := decryptPlayIntegrityToken(req)
	if err != nil {
		inst.Logger().Debugf("cannot decrypt the play integrity token: %s", err)
		return fmt.Errorf("cannot parse attestation: %s", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return errors.New("invalid claims type")
	}
	inst.Logger().Debugf("checkPlayIntegrityAttestation claims = %#v", claims)

	nonce, ok := getFromClaims(claims, "requestDetails.nonce").(string)
	if !ok || len(nonce) == 0 {
		return errors.New("missing nonce")
	}
	if req.Challenge != nonce {
		return errors.New("invalid nonce")
	}

	if err := checkPlayIntegrityPackageName(claims); err != nil {
		return err
	}
	if err := checkPlayIntegrityCertificateDigest(claims); err != nil {
		return err
	}
	return nil
}

// CheckPlayIntegrityAttestationForTestingPurpose is only used for testing
// purpose. It is a simplified version of checkPlayIntegrityAttestation. In
// particular, it doesn't return an error for invalid package name with a test
// attestation.
func CheckPlayIntegrityAttestationForTestingPurpose(req AttestationRequest) error {
	token, err := decryptPlayIntegrityToken(req)
	if err != nil {
		return fmt.Errorf("cannot parse attestation: %s", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return errors.New("invalid claims type")
	}

	nonce, ok := getFromClaims(claims, "requestDetails.nonce").(string)
	if !ok || len(nonce) == 0 {
		return errors.New("missing nonce")
	}
	if req.Challenge != nonce {
		return errors.New("invalid nonce")
	}
	return nil
}

func decryptPlayIntegrityToken(req AttestationRequest) (*jwt.Token, error) {
	lastErr := errors.New("no decryption key")
	for _, key := range config.GetConfig().Flagship.PlayIntegrityDecryptionKeys {
		decrypted, err := decryptPlayIntegrityJWE(req.Attestation, key)
		if err == nil {
			return parsePlayIntegrityToken(decrypted)
		}
		lastErr = err
	}
	return nil, lastErr
}

func decryptPlayIntegrityJWE(attestation string, rawKey string) ([]byte, error) {
	parts := strings.Split(attestation, ".")
	if len(parts) != 5 {
		return nil, errors.New("invalid integrity token")
	}
	header := []byte(parts[0])
	encryptedKey, err := base64.RawURLEncoding.DecodeString(parts[1])
	// AES Key wrap works with 64 bits block, and the wrapped version has n+1
	// blocks (for integrity check). The kek key is 256bits, thus the
	// encryptedKey is 320bits => 40bytes.
	if err != nil || len(encryptedKey) != 40 {
		return nil, fmt.Errorf("invalid encrypted key: %w", err)
	}
	initVector, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid initialization vector: %w", err)
	}
	cipherText, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		return nil, fmt.Errorf("invalid ciphertext: %w", err)
	}
	authTag, err := base64.RawURLEncoding.DecodeString(parts[4])
	if err != nil || len(authTag) != 16 { // GCM uses 128bits => 16bytes
		return nil, fmt.Errorf("invalid authentication tag: %w", err)
	}

	kek, err := base64.StdEncoding.DecodeString(rawKey) // kek means Key-encryption key, cf RFC-3394
	if err != nil {
		return nil, fmt.Errorf("invalid decryption key: %w", err)
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, fmt.Errorf("invalid decryption key: %w", err)
	}
	contentKey, err := crypto.UnwrapA256KW(block, encryptedKey)
	if err != nil {
		return nil, fmt.Errorf("cannot unwrap the key: %w", err)
	}
	if len(contentKey) != 32 { // AES256 means 256bits => 32bytes
		return nil, fmt.Errorf("invalid encrypted key: %w", err)
	}

	cek, err := aes.NewCipher(contentKey)
	if err != nil {
		return nil, fmt.Errorf("cannot load the cek: %w", err)
	}
	aesgcm, err := cipher.NewGCM(cek)
	if err != nil {
		return nil, fmt.Errorf("cannot initialize AES-GCM: %w", err)
	}
	if len(initVector) != aesgcm.NonceSize() {
		return nil, fmt.Errorf("invalid initialization vector: %w", err)
	}
	decrypted, err := aesgcm.Open(nil, initVector, append(cipherText, authTag...), header)
	if err != nil {
		return nil, fmt.Errorf("cannot decrypt: %w", err)
	}

	return decrypted, nil
}

func parsePlayIntegrityToken(decrypted []byte) (*jwt.Token, error) {
	lastErr := errors.New("no verification key")
	for _, key := range config.GetConfig().Flagship.PlayIntegrityVerificationKeys {
		token, err := parsePlayIntegrityJWT(decrypted, key)
		if err == nil {
			return token, err
		}
		lastErr = err
	}
	return nil, lastErr
}

func parsePlayIntegrityJWT(decrypted []byte, rawKey string) (*jwt.Token, error) {
	return jwt.Parse(string(decrypted), func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		key, err := base64.StdEncoding.DecodeString(rawKey)
		if err != nil {
			return nil, fmt.Errorf("invalid verification key: %w", err)
		}
		pubKey, err := x509.ParsePKIXPublicKey(key)
		if err != nil {
			return nil, fmt.Errorf("invalid verification key: %w", err)
		}
		return pubKey, nil
	})
}

func checkPlayIntegrityPackageName(claims jwt.MapClaims) error {
	packageName, ok := getFromClaims(claims, "appIntegrity.packageName").(string)
	if !ok || len(packageName) == 0 {
		return errors.New("missing appIntegrity.packageName")
	}
	names := config.GetConfig().Flagship.APKPackageNames
	for _, name := range names {
		if name == packageName {
			return nil
		}
	}
	return fmt.Errorf("%s is not the package name of the flagship app", packageName)
}

func checkPlayIntegrityCertificateDigest(claims jwt.MapClaims) error {
	certDigest, ok := getFromClaims(claims, "appIntegrity.certificateSha256Digest").([]interface{})
	if !ok || len(certDigest) == 0 {
		return errors.New("missing appIntegrity.certificateSha256Digest")
	}
	digests := config.GetConfig().Flagship.APKCertificateDigests
	for _, digest := range digests {
		if digest == certDigest[0] {
			return nil
		}
		// XXX Google was using standard base64 for SafetyNet, but the safe-URL
		// variant for Play Integrity...
		urlSafeDigest := strings.TrimRight(digest, "=")
		urlSafeDigest = strings.ReplaceAll(urlSafeDigest, "+", "-")
		urlSafeDigest = strings.ReplaceAll(urlSafeDigest, "/", "_")
		if urlSafeDigest == certDigest[0] {
			return nil
		}
	}
	logger.WithNamespace("oauth").
		Debugf("Invalid certificate digest, expected %s, got %s", digests[0], certDigest[0])
	return errors.New("invalid certificate digest")
}

func getFromClaims(claims jwt.MapClaims, key string) interface{} {
	parts := strings.Split(key, ".")
	var obj interface{} = map[string]interface{}(claims)
	for _, part := range parts {
		m, ok := obj.(map[string]interface{})
		if !ok {
			return nil
		}
		obj = m[part]
	}
	return obj
}
