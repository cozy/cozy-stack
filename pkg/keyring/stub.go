package keyring

import (
	"fmt"

	"github.com/cozy/cozy-stack/pkg/utils"
)

// Stub is a minimal *UNSECURE* implementation of [Keyring].
//
// As the credentials should remain the same between several
// executions of the stack, we are using some credentials generated
// with a seed defined at build time. It is obviously not a good idea
// from a security point of view, and it should not be used to store
// sensible data. This implem is not safe and should never be used in
// production.
type Stub struct {
	credsEncryptor *NACLKey
	credsDecryptor *NACLKey
}

// NewStub instantiate a new [Stub].
func NewStub() (*Stub, error) {
	r := utils.NewSeededRand(42)

	credsEncryptor, credsDecryptor, err := GenerateKeyPair(r)
	if err != nil {
		return nil, fmt.Errorf("failed to generate NACL key pair: %w", err)
	}

	return &Stub{credsEncryptor, credsDecryptor}, nil
}

func (s *Stub) CredentialsEncryptorKey() *NACLKey {
	return s.credsEncryptor
}

func (s *Stub) CredentialsDecryptorKey() *NACLKey {
	return s.credsDecryptor
}
