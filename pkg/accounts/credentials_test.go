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
	encryptedCreds1, err := EncryptCredentials("me@mycozy.cloud", "fzEE6HFWsSp8jP")
	if !assert.NoError(t, err) {
		return
	}
	encryptedCreds2, err := EncryptCredentials("me@mycozy.cloud", "fzEE6HFWsSp8jP")
	if !assert.NoError(t, err) {
		return
	}
	assert.False(t, bytes.Equal(encryptedCreds1, encryptedCreds2))

	{
		login, password, err := DecryptCredentials(encryptedCreds1)
		if !assert.NoError(t, err) {
			return
		}
		assert.Equal(t, "me@mycozy.cloud", login)
		assert.Equal(t, "fzEE6HFWsSp8jP", password)
	}
	{
		login, password, err := DecryptCredentials(encryptedCreds2)
		if !assert.NoError(t, err) {
			return
		}
		assert.Equal(t, "me@mycozy.cloud", login)
		assert.Equal(t, "fzEE6HFWsSp8jP", password)
	}
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

func TestRandomBitFlips(t *testing.T) {
	original, err := EncryptCredentials("toto@titi.com", "X3hVYLJLRiUyCs")
	if !assert.NoError(t, err) {
		return
	}

	flipped := make([]byte, len(original))
	copy(flipped, original)
	login, passwd, err := DecryptCredentials(flipped)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "toto@titi.com", login)
	assert.Equal(t, "X3hVYLJLRiUyCs", passwd)

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 100000; i++ {
		copy(flipped, original)
		flipsLen := rng.Intn(30) + 1
		flipsSet := make([]int, 0, flipsLen)
		for len(flipsSet) < flipsLen {
			flipValue := rng.Intn(len(original) * 8)
			flipFound := false
			for j := 0; j < len(flipsSet); j++ {
				if flipsSet[j] == flipValue {
					flipFound = true
					break
				}
			}
			if !flipFound {
				flipsSet = append(flipsSet, flipValue)
			}
		}
		for _, flipValue := range flipsSet {
			mask := byte(0x1 << uint(flipValue%8))
			flipped[flipValue/8] ^= mask
		}
		_, _, err := DecryptCredentials(flipped)
		if !assert.Error(t, err) {
			t.Fatalf("Failed with flips %v", flipsSet)
			return
		}
	}
}

func TestMain(m *testing.M) {
	config.UseTestFile()
	os.Exit(m.Run())
}
