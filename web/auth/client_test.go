package auth

import (
	"testing"

	"github.com/cozy/cozy-stack/crypto"
	"github.com/cozy/cozy-stack/instance"
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/stretchr/testify/assert"
)

var secret = crypto.GenerateRandomBytes(64)
var i = &instance.Instance{
	OAuthSecret: secret,
	Domain:      "test-jwt.example.org",
}
var c = &Client{
	CouchID: "my-client-id",
}

func TestCreateJWT(t *testing.T) {
	tokenString, err := c.CreateJWT(i, "test", "foo:read")
	assert.NoError(t, err)

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		_, ok := token.Method.(*jwt.SigningMethodHMAC)
		assert.True(t, ok, "The signing method should be HMAC")
		return i.OAuthSecret, nil
	})
	assert.NoError(t, err)
	assert.True(t, token.Valid)

	claims, ok := token.Claims.(jwt.MapClaims)
	assert.True(t, ok, "Claims can be parsed as standard claims")
	assert.Equal(t, "test", claims["aud"])
	assert.Equal(t, "test-jwt.example.org", claims["iss"])
	assert.Equal(t, "my-client-id", claims["sub"])
	assert.Equal(t, "foo:read", claims["scope"])
}

func TestParseJWT(t *testing.T) {
	tokenString, err := c.CreateJWT(i, "refresh", "foo:read")
	assert.NoError(t, err)

	claims, ok := c.ValidRefreshToken(i, tokenString)
	assert.True(t, ok, "The token must be valid")
	assert.Equal(t, "refresh", claims.Audience)
	assert.Equal(t, "test-jwt.example.org", claims.Issuer)
	assert.Equal(t, "my-client-id", claims.Subject)
	assert.Equal(t, "foo:read", claims.Scope)
}

func TestParseJWTInvalidAudience(t *testing.T) {
	tokenString, err := c.CreateJWT(i, "access", "foo:read")
	assert.NoError(t, err)
	_, ok := c.ValidRefreshToken(i, tokenString)
	assert.False(t, ok, "The token should be invalid")
}

func TestParseJWTInvalidIssuer(t *testing.T) {
	other := &instance.Instance{
		OAuthSecret: i.OAuthSecret,
		Domain:      "other.example.com",
	}
	tokenString, err := c.CreateJWT(other, "refresh", "foo:read")
	assert.NoError(t, err)
	_, ok := c.ValidRefreshToken(i, tokenString)
	assert.False(t, ok, "The token should be invalid")
}

func TestParseJWTInvalidSubject(t *testing.T) {
	other := &Client{
		CouchID: "my-other-client",
	}
	tokenString, err := other.CreateJWT(i, "refresh", "foo:read")
	assert.NoError(t, err)
	_, ok := c.ValidRefreshToken(i, tokenString)
	assert.False(t, ok, "The token should be invalid")
}
