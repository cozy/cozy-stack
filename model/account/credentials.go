package account

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/keymgmt"
	"golang.org/x/crypto/nacl/box"
)

const cipherHeader = "nacl"
const nonceLen = 24
const plainPrefixLen = 4

var (
	errCannotDecrypt  = errors.New("accounts: cannot decrypt credentials")
	errCannotEncrypt  = errors.New("accounts: cannot encrypt credentials")
	errBadCredentials = errors.New("accounts: bad credentials")
)

// EncryptCredentialsWithKey takes a login / password and encrypts their values using
// the vault public key.
func EncryptCredentialsWithKey(encryptorKey *keymgmt.NACLKey, login, password string) (string, error) {
	if encryptorKey == nil {
		return "", errCannotEncrypt
	}

	loginLen := len(login)

	// make a buffer containing the length of the login in bigendian over 4
	// bytes, followed by the login and password contatenated.
	creds := make([]byte, plainPrefixLen+loginLen+len(password))

	// put the length of login in the first 4 bytes
	binary.BigEndian.PutUint32(creds[0:], uint32(loginLen))

	// copy the concatenation of login + password in the end
	copy(creds[plainPrefixLen:], login)
	copy(creds[plainPrefixLen+loginLen:], password)

	var nonce [nonceLen]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		panic(err)
	}

	encryptedOut := make([]byte, len(cipherHeader)+len(nonce))
	copy(encryptedOut[0:], cipherHeader)
	copy(encryptedOut[len(cipherHeader):], nonce[:])

	encryptedCreds := box.Seal(encryptedOut, creds, &nonce, encryptorKey.PublicKey(), encryptorKey.PrivateKey())
	return base64.StdEncoding.EncodeToString(encryptedCreds), nil
}

// EncryptCredentialsData takes any json encodable data and encode and encrypts
// it using the vault public key.
func EncryptCredentialsData(data interface{}) (string, error) {
	encryptorKey := config.GetVault().CredentialsEncryptorKey()
	if encryptorKey == nil {
		return "", errCannotEncrypt
	}
	buf, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	cipher, err := EncryptBufferWithKey(encryptorKey, buf)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(cipher), nil
}

// EncryptBufferWithKey encrypts the given bytee buffer with the specified encryption
// key.
func EncryptBufferWithKey(encryptorKey *keymgmt.NACLKey, buf []byte) ([]byte, error) {
	var nonce [nonceLen]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		panic(err)
	}

	encryptedOut := make([]byte, len(cipherHeader)+len(nonce))
	copy(encryptedOut[0:], cipherHeader)
	copy(encryptedOut[len(cipherHeader):], nonce[:])

	encryptedCreds := box.Seal(encryptedOut, buf, &nonce, encryptorKey.PublicKey(), encryptorKey.PrivateKey())
	return encryptedCreds, nil
}

// EncryptCredentials encrypts the given credentials with the specified encryption
// key.
func EncryptCredentials(login, password string) (string, error) {
	encryptorKey := config.GetVault().CredentialsEncryptorKey()
	if encryptorKey == nil {
		return "", errCannotEncrypt
	}
	return EncryptCredentialsWithKey(encryptorKey, login, password)
}

// DecryptCredentials takes an encrypted credentials, constiting of a login /
// password pair, and decrypts it using the vault private key.
func DecryptCredentials(encryptedData string) (login, password string, err error) {
	decryptorKey := config.GetVault().CredentialsDecryptorKey()
	if decryptorKey == nil {
		return "", "", errCannotDecrypt
	}
	encryptedBuffer, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return "", "", errCannotDecrypt
	}
	return DecryptCredentialsWithKey(decryptorKey, encryptedBuffer)
}

// DecryptCredentialsWithKey takes an encrypted credentials, constiting of a
// login / password pair, and decrypts it using the given private key.
func DecryptCredentialsWithKey(decryptorKey *keymgmt.NACLKey, encryptedCreds []byte) (login, password string, err error) {
	// check the cipher text starts with the cipher header
	if !bytes.HasPrefix(encryptedCreds, []byte(cipherHeader)) {
		return "", "", errBadCredentials
	}
	// skip the cipher header
	encryptedCreds = encryptedCreds[len(cipherHeader):]

	// check the encrypted creds contains the space for the nonce as prefix
	if len(encryptedCreds) < nonceLen {
		return "", "", errBadCredentials
	}

	// extrct the nonce from the first 24 bytes
	var nonce [nonceLen]byte
	copy(nonce[:], encryptedCreds[:nonceLen])

	// skip the nonce
	encryptedCreds = encryptedCreds[nonceLen:]

	// decrypt the cipher text and check that the plain text is more the 4 bytes
	// long, to contain the login length
	creds, ok := box.Open(nil, encryptedCreds, &nonce, decryptorKey.PublicKey(), decryptorKey.PrivateKey())
	if !ok {
		return "", "", errBadCredentials
	}

	// extract login length from 4 first bytes
	loginLen := int(binary.BigEndian.Uint32(creds[0:]))

	// skip login length
	creds = creds[plainPrefixLen:]

	// check credentials contains enough space to contain at least the login
	if len(creds) < loginLen {
		return "", "", errBadCredentials
	}

	// split the credentials into login / password
	return string(creds[:loginLen]), string(creds[loginLen:]), nil
}

// DecryptCredentialsData takes an encryted buffer and decrypts and decode its
// content.
func DecryptCredentialsData(encryptedData string) (interface{}, error) {
	decryptorKey := config.GetVault().CredentialsDecryptorKey()
	if decryptorKey == nil {
		return nil, errCannotDecrypt
	}
	encryptedBuffer, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return nil, errCannotDecrypt
	}
	plainBuffer, err := DecryptBufferWithKey(decryptorKey, encryptedBuffer)
	if err != nil {
		return nil, err
	}
	var data interface{}
	if err = json.Unmarshal(plainBuffer, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// DecryptBufferWithKey takes an encrypted buffer and decrypts it using the
// given private key.
func DecryptBufferWithKey(decryptorKey *keymgmt.NACLKey, encryptedBuffer []byte) ([]byte, error) {
	// check the cipher text starts with the cipher header
	if !bytes.HasPrefix(encryptedBuffer, []byte(cipherHeader)) {
		return nil, errBadCredentials
	}

	// skip the cipher header
	encryptedBuffer = encryptedBuffer[len(cipherHeader):]

	// check the encrypted creds contains the space for the nonce as prefix
	if len(encryptedBuffer) < nonceLen {
		return nil, errBadCredentials
	}

	// extrct the nonce from the first 24 bytes
	var nonce [nonceLen]byte
	copy(nonce[:], encryptedBuffer[:nonceLen])

	// skip the nonce
	encryptedBuffer = encryptedBuffer[nonceLen:]

	// decrypt the cipher text and check that the plain text is more the 4 bytes
	// long, to contain the login length
	plainBuffer, ok := box.Open(nil, encryptedBuffer, &nonce, decryptorKey.PublicKey(), decryptorKey.PrivateKey())
	if !ok {
		return nil, errBadCredentials
	}

	return plainBuffer, nil
}
