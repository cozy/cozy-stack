package crypto

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

var pass = []byte("This is secret")

var oldhash = []byte("scrypt$16384$8$1$172721d78c4dc2d6$d74e5214536e31193b5300d6e162e2fdcb9f0c1bffc5be446b93edee65a0570d")
var goodhash = []byte("scrypt$16384$8$1$615705b4db4b15c8c4a54f906f3ba032$a8ea0f7c37c40dd314b9bab56ea50030ce10591e70e90d4a0a5346e849f2c7c4")
var badhash = []byte("scrypt$16384$8$1$3a371fe057cef0063d01fce866acb989$3228bf807f307badbadbadbadbadbadbadbad09fc011f5859ad6a1504de56455")

func TestGenerateFromPassphrase(t *testing.T) {
	val, err := GenerateFromPassphrase(pass)
	assert.NoError(t, err)
	assert.Equal(t, 5, bytes.Count(val, sep), "hash should have 6 parts")
	algo := string(bytes.Split(val, sep)[0])
	assert.Equal(t, "scrypt", algo, "hash should contain algo")
}

func TestCompareGoodHashAndPassphrase(t *testing.T) {
	_, err := CompareHashAndPassphrase(goodhash, pass)
	assert.NoError(t, err)
}

func TestCompareBadHashAndPassphrase(t *testing.T) {
	_, err := CompareHashAndPassphrase(badhash, pass)
	assert.Error(t, err)
}

func TestUpdateHashNoUpdate(t *testing.T) {
	needUpdate, err := CompareHashAndPassphrase(goodhash, pass)
	assert.NoError(t, err)
	assert.False(t, needUpdate)
}

func TestUpdateHashNeedUpdate(t *testing.T) {
	needUpdate, err := CompareHashAndPassphrase(oldhash, pass)
	assert.NoError(t, err)
	assert.True(t, needUpdate)
}
