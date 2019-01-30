package sessions

import (
	"testing"
	"time"

	jwt "gopkg.in/dgrijalva/jwt-go.v3"

	"github.com/cozy/cozy-stack/pkg/instance"

	"github.com/stretchr/testify/assert"
)

var delegatedInst *instance.Instance

func TestGoodCheckDelegatedJWT(t *testing.T) {
	// token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJzcnV0aSIsIm5hbWUiOiJleHRlcm5hbC5ub3RteWNvenkuY29tIiwiaWF0IjoxNTE2MjM5MDIyLCJleHAiOjE2MDc3MzEyMDAsImVtYWlsIjoic3J1dGlAYWMtcmVubmVzLmZyIiwiY29kZSI6InN0dWRlbnQifQ.mHYke9WhLeggCmv7RdqoAWtQVT45KwT3bz_-fPMcuMc"
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, ExternalClaims{
		StandardClaims: jwt.StandardClaims{
			Subject:   "sruti",
			IssuedAt:  time.Now().Unix(),
			ExpiresAt: time.Now().Add(time.Hour).Unix(),
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
		StandardClaims: jwt.StandardClaims{
			Subject:   "sruti",
			IssuedAt:  time.Now().Add(-2 * time.Hour).Unix(),
			ExpiresAt: time.Now().Add(-1 * time.Hour).Unix(),
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
		StandardClaims: jwt.StandardClaims{
			Subject:   "sruti",
			IssuedAt:  time.Now().Unix(),
			ExpiresAt: time.Now().Add(time.Hour).Unix(),
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
