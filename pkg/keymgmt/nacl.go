package keymgmt

import (
	"bytes"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"

	"golang.org/x/crypto/nacl/box"
)

const (
	naclKeyBlockType = "NACL KEY"

	naclKeyLen = 32
)

var errNACLBadKey = errors.New("keymgmt: bad nacl key")

// NACLKey contains a NACL crypto box keypair.
type NACLKey struct {
	publicKey  *[32]byte
	privateKey *[32]byte
}

// PublicKey returns the public part of the keypair.
func (n *NACLKey) PublicKey() *[32]byte {
	return n.publicKey
}

// PrivateKey returns the private part of the keypair.
func (n *NACLKey) PrivateKey() *[32]byte {
	return n.privateKey
}

// GenerateKeyPair returns a couple keypairs that can be used for asymmetric
// encryption/decryption using nacl crypto box API.
func GenerateKeyPair() (encryptorKey *NACLKey, decryptorKey *NACLKey, err error) {
	senderPublicKey, senderPrivateKey, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return
	}
	receiverPublicKey, receiverPrivateKey, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return
	}
	encryptorKey = &NACLKey{
		publicKey:  receiverPublicKey,
		privateKey: senderPrivateKey,
	}
	decryptorKey = &NACLKey{
		publicKey:  senderPublicKey,
		privateKey: receiverPrivateKey,
	}
	return
}

// GenerateEncodedNACLKeyPair returns to byte slice containing the encoded
// values of the couple of keypairs freshly generated.
func GenerateEncodedNACLKeyPair() (marshaledEncryptorKey []byte, marshaledDecryptorKey []byte, err error) {
	encryptorKey, decryptorKey, err := GenerateKeyPair()
	if err != nil {
		return
	}
	marshaledEncryptorKey = MarshalNACLKey(encryptorKey)
	marshaledDecryptorKey = MarshalNACLKey(decryptorKey)
	return
}

// UnmarshalNACLKey takes and encoded value of a keypair and unmarshal its
// value, returning the associated key.
func UnmarshalNACLKey(marshaledKey []byte) (key *NACLKey, err error) {
	keys, err := unmarshalPEMBlock(marshaledKey, naclKeyBlockType)
	if err != nil {
		return
	}
	if len(keys) != 2*naclKeyLen {
		err = errNACLBadKey
		return
	}
	publicKey := new([naclKeyLen]byte)
	privateKey := new([naclKeyLen]byte)
	copy(publicKey[:], keys[:])
	copy(privateKey[:], keys[naclKeyLen:])
	key = &NACLKey{
		publicKey:  publicKey,
		privateKey: privateKey,
	}
	return
}

// MarshalNACLKey takes a key and returns its encoded version.
func MarshalNACLKey(key *NACLKey) []byte {
	keyBytes := make([]byte, 2*naclKeyLen)
	copy(keyBytes[:], key.publicKey[:])
	copy(keyBytes[naclKeyLen:], key.privateKey[:])
	return pem.EncodeToMemory(&pem.Block{
		Type:  naclKeyBlockType,
		Bytes: keyBytes,
	})
}

func unmarshalPEMBlock(keyBytes []byte, blockType string) ([]byte, error) {
	if !bytes.HasPrefix(keyBytes, []byte("-----BEGIN")) {
		return nil, fmt.Errorf("keymgmt: bad PEM block header")
	}
	block, _ := pem.Decode(keyBytes)
	if block == nil {
		return nil, fmt.Errorf("keymgmt: failed to parse PEM block containing the public key")
	}
	if block.Type != blockType {
		return nil, fmt.Errorf(`keymgmt: bad PEM block type, got %q expecting %q`,
			block.Type, blockType)
	}
	return block.Bytes, nil
}
