package session

import (
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/instance"

	jwt "github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
)

var delegatedInst *instance.Instance

func TestGoodCheckDelegatedJWT(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, ExternalClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "sruti",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Name:  "external.notmycozy.net",
		Email: "sruti@external.notmycozy.net",
		Code:  "student",
	})
	signed, err := token.SignedString(JWTSecret)
	assert.NoError(t, err)
	err = CheckDelegatedJWT(delegatedInst, signed)
	assert.NoError(t, err)
}

func TestBadExpiredCheckDelegatedJWT(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, ExternalClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "sruti",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
		},
		Name:  "external.notmycozy.net",
		Email: "sruti@external.notmycozy.net",
		Code:  "student",
	})
	signed, err := token.SignedString(JWTSecret)
	assert.NoError(t, err)
	err = CheckDelegatedJWT(delegatedInst, signed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestBadIssuerCheckDelegatedJWT(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, ExternalClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "sruti",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Name:  "evil.notmycozy.net",
		Email: "sruti@external.notmycozy.net",
		Code:  "student",
	})
	signed, err := token.SignedString(JWTSecret)
	assert.NoError(t, err)
	err = CheckDelegatedJWT(delegatedInst, signed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Issuer")
}
