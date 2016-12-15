package auth

import (
	"testing"

	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/stretchr/testify/assert"
)

var instanceSecret = crypto.GenerateRandomBytes(64)
var in = &instance.Instance{
	OAuthSecret: instanceSecret,
	Domain:      "test-jwt.example.org",
}
var c = &Client{
	CouchID: "my-client-id",
}

func TestCreateJWT(t *testing.T) {
	tokenString, err := c.CreateJWT(in, "test", "foo:read")
	assert.NoError(t, err)

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		_, ok := token.Method.(*jwt.SigningMethodHMAC)
		assert.True(t, ok, "The signing method should be HMAC")
		return in.OAuthSecret, nil
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
	tokenString, err := c.CreateJWT(in, "refresh", "foo:read")
	assert.NoError(t, err)

	claims, ok := c.ValidToken(in, RefreshTokenAudience, tokenString)
	assert.True(t, ok, "The token must be valid")
	assert.Equal(t, "refresh", claims.Audience)
	assert.Equal(t, "test-jwt.example.org", claims.Issuer)
	assert.Equal(t, "my-client-id", claims.Subject)
	assert.Equal(t, "foo:read", claims.Scope)
}

func TestParseJWTInvalidAudience(t *testing.T) {
	tokenString, err := c.CreateJWT(in, "access", "foo:read")
	assert.NoError(t, err)
	_, ok := c.ValidToken(in, RefreshTokenAudience, tokenString)
	assert.False(t, ok, "The token should be invalid")
}

func TestParseJWTInvalidIssuer(t *testing.T) {
	other := &instance.Instance{
		OAuthSecret: in.OAuthSecret,
		Domain:      "other.example.com",
	}
	tokenString, err := c.CreateJWT(other, "refresh", "foo:read")
	assert.NoError(t, err)
	_, ok := c.ValidToken(in, RefreshTokenAudience, tokenString)
	assert.False(t, ok, "The token should be invalid")
}

func TestParseJWTInvalidSubject(t *testing.T) {
	other := &Client{
		CouchID: "my-other-client",
	}
	tokenString, err := other.CreateJWT(in, "refresh", "foo:read")
	assert.NoError(t, err)
	_, ok := c.ValidToken(in, RefreshTokenAudience, tokenString)
	assert.False(t, ok, "The token should be invalid")
}
