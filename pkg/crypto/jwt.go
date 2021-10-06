package crypto

import (
	"errors"
	"fmt"
	"time"

	jwt "github.com/golang-jwt/jwt/v4"
)

// StandardClaims are a structured version of the JWT Claims Set, as referenced at
// https://datatracker.ietf.org/doc/html/rfc7519#section-4. They do not follow the
// specification exactly, since they were based on an earlier draft of the
// specification and not updated. The main difference is that they only
// support integer-based date fields and singular audiences.
type StandardClaims struct {
	Audience  string `json:"aud,omitempty"`
	ExpiresAt int64  `json:"exp,omitempty"`
	IssuedAt  int64  `json:"iat,omitempty"`
	Issuer    string `json:"iss,omitempty"`
	NotBefore int64  `json:"nbf,omitempty"`
	Subject   string `json:"sub,omitempty"`
}

// Valid validates time based claims "exp, iat, nbf". There is no accounting
// for clock skew. As well, if any of the above claims are not in the token, it
// will still be considered a valid claim.
func (claims StandardClaims) Valid() error {
	now := time.Now().Unix()

	if claims.IssuedAt > now {
		return fmt.Errorf("token used before issued")
	}

	// The claims below are optional, by default, so if they are set to the
	// default value in Go, let's not fail the verification for them.
	if claims.ExpiresAt > 0 && claims.ExpiresAt < now {
		return fmt.Errorf("token is expired by %v", now-claims.ExpiresAt)
	}
	if claims.NotBefore > 0 && claims.NotBefore > now {
		return fmt.Errorf("token is not valid yet")
	}

	return nil
}

// SigningMethod is the algorithm choosed for signing JWT.
// Currently, it is HMAC-SHA-512
var SigningMethod = jwt.SigningMethodHS512

// NewJWT creates a JWT token with the given claims,
// and signs it with the secret
func NewJWT(secret []byte, claims jwt.Claims) (string, error) {
	token := jwt.NewWithClaims(SigningMethod, claims)
	return token.SignedString(secret)
}

// ParseJWT parses a string and checkes that is a valid JSON Web Token
func ParseJWT(tokenString string, keyFunc jwt.Keyfunc, claims jwt.Claims) error {
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return keyFunc(token)
	})
	if err != nil {
		return err
	}
	if !token.Valid {
		return errors.New("Invalid JSON Web Token")
	}
	return nil
}
