package crypto

import (
	"testing"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/stretchr/testify/assert"
)

func TestNewJWT(t *testing.T) {
	secret, err := GenerateRandomBytes(64)
	assert.NoError(t, err)
	tokenString, err := NewJWT(secret, jwt.StandardClaims{
		Audience: "test",
		Issuer:   "example.org",
		IssuedAt: time.Now().Unix(),
		Subject:  "cozy.io",
	})
	assert.NoError(t, err)

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		_, ok := token.Method.(*jwt.SigningMethodHMAC)
		assert.True(t, ok, "The signing method should be HMAC")
		return secret, nil
	})
	assert.True(t, token.Valid)

	claims, ok := token.Claims.(jwt.MapClaims)
	assert.True(t, ok, "Claims can be parsed as standard claims")
	assert.Equal(t, "test", claims["aud"])
	assert.Equal(t, "example.org", claims["iss"])
	assert.Equal(t, "cozy.io", claims["sub"])
}
