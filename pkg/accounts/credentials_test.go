package accounts

import (
	"bytes"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestEncryptDecrytCredentials(t *testing.T) {
	encryptedCreds, err := EncryptCredentials("me@mycozy.cloud", "fzEE6HFWsSp8jP")
	if !assert.NoError(t, err) {
		return
	}

	login, password, err := DecryptCredentials(encryptedCreds)
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, "me@mycozy.cloud", login)
	assert.Equal(t, "fzEE6HFWsSp8jP", password)
}

func TestEncryptDecrytUTF8Credentials(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 1024; i++ {
		login := string(crypto.GenerateRandomBytes(rng.Intn(256)))
		password := string(crypto.GenerateRandomBytes(rng.Intn(256)))

		encryptedCreds, err := EncryptCredentials(login, password)
		if !assert.NoError(t, err) {
			return
		}

		loginDec, passwordDec, err := DecryptCredentials(encryptedCreds)
		if !assert.NoError(t, err) {
			return
		}

		assert.Equal(t, loginDec, login)
		assert.Equal(t, passwordDec, password)
	}

	for i := 0; i < 1024; i++ {
		login := string(utils.RandomString(rng.Intn(256)))
		password := string(utils.RandomString(rng.Intn(256)))

		encryptedCreds, err := EncryptCredentials(login, password)
		if !assert.NoError(t, err) {
			return
		}

		loginDec, passwordDec, err := DecryptCredentials(encryptedCreds)
		if !assert.NoError(t, err) {
			return
		}

		assert.True(t, bytes.Equal(encryptedCreds[:len(cipherHeader)], []byte(cipherHeader)))
		assert.Equal(t, loginDec, login)
		assert.Equal(t, passwordDec, password)
	}
}

func TestDecryptCredentialsRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 1024; i++ {
		encrypted := crypto.GenerateRandomBytes(rng.Intn(256))
		_, _, err := DecryptCredentials(encrypted)
		assert.Error(t, err)
	}
	for i := 0; i < 1024; i++ {
		encrypted := crypto.GenerateRandomBytes(rng.Intn(256))
		encryptedWithHeader := make([]byte, len(cipherHeader)+len(encrypted))
		copy(encryptedWithHeader[0:], cipherHeader)
		copy(encryptedWithHeader[len(cipherHeader):], encrypted)
		_, _, err := DecryptCredentials(encryptedWithHeader)
		assert.Error(t, err)
	}
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	os.Exit(m.Run())
}
