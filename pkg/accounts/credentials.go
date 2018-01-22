package accounts

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"

	"github.com/cozy/cozy-stack/pkg/config"
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

// EncryptCredentials takes a login / password and encrypts their values using
// the vault public key.
func EncryptCredentials(login, password string) ([]byte, error) {
	encryptorKey := config.GetVault().CredentialsEncryptorKey()
	if encryptorKey == nil {
		return nil, errCannotEncrypt
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
	return encryptedCreds, nil
}

// DecryptCredentials takes an encrypted credentials, constiting of a login /
// password pair, and decrypts it using the vault private key.
func DecryptCredentials(encryptedCreds []byte) (login, password string, err error) {
	decryptorKey := config.GetVault().CredentialsDecryptorKey()
	if decryptorKey == nil {
		return "", "", errCannotDecrypt
	}

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
