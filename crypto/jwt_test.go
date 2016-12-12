package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

func TestNewJWT(t *testing.T) {
	secret := GenerateRandomBytes(64)
	tokenString, err := NewJWT(secret, jwt.StandardClaims{
		Audience: "test",
		Issuer:   "example.org",
		IssuedAt: Timestamp(),
		Subject:  "cozy.io",
	})
	assert.NoError(t, err)

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		_, ok := token.Method.(*jwt.SigningMethodHMAC)
		assert.True(t, ok, "The signing method should be HMAC")
		return secret, nil
	})
	assert.NoError(t, err)
	assert.True(t, token.Valid)

	claims, ok := token.Claims.(jwt.MapClaims)
	assert.True(t, ok, "Claims can be parsed as standard claims")
	assert.Equal(t, "test", claims["aud"])
	assert.Equal(t, "example.org", claims["iss"])
	assert.Equal(t, "cozy.io", claims["sub"])
}
