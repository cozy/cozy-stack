package keyring

import (
	"errors"
	"fmt"
	"os"
)

var (
	ErrFieldRequired = errors.New("field required")
)

// Keyring handle the encryption/decryption keys
type Keyring interface {
	// CredentialsEncryptorKey returns the key used to encrypt credentials values,
	// stored in accounts.
	CredentialsEncryptorKey() *NACLKey
	// CredentialsDecryptorKey returns the key used to decrypt credentials values,
	// stored in accounts.
	CredentialsDecryptorKey() *NACLKey
}

// Config used to setup a [Keyring] service.
type Config struct {
	EncryptorKeyPath string `mapstructure:"credentials_encryptor_key"`
	DecryptorKeyPath string `mapstructure:"credentials_decryptor_key"`
}

// Service contains security keys used for various encryption or signing of
// critical assets.
type Service struct {
	credsEncryptor *NACLKey
	credsDecryptor *NACLKey
}

func NewFromConfig(conf Config) (Keyring, error) {
	if conf.DecryptorKeyPath == "" || conf.EncryptorKeyPath == "" {
		return NewStub()
	}

	return NewService(conf)
}

// NewService instantiate a new [Keyring].
func NewService(conf Config) (*Service, error) {
	if conf.EncryptorKeyPath == "" {
		return nil, fmt.Errorf("credentials_encryptor_key: %w", ErrFieldRequired)
	}

	if conf.DecryptorKeyPath == "" {
		return nil, fmt.Errorf("credentials_decryptor_key: %w", ErrFieldRequired)
	}

	credsEncryptor, err := decodeKeyFromPath(conf.EncryptorKeyPath)
	if err != nil {
		return nil, err
	}

	credsDecryptor, err := decodeKeyFromPath(conf.DecryptorKeyPath)
	if err != nil {
		return nil, err
	}

	return &Service{credsEncryptor, credsDecryptor}, nil
}

func (s *Service) CredentialsEncryptorKey() *NACLKey {
	return s.credsEncryptor
}

func (s *Service) CredentialsDecryptorKey() *NACLKey {
	return s.credsDecryptor
}

func decodeKeyFromPath(path string) (*NACLKey, error) {
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %q: %w", path, err)
	}

	creds, err := UnmarshalNACLKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal NACL key: %w", err)
	}

	return creds, nil
}
