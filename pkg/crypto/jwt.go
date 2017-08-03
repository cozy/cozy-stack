package crypto

import (
	"errors"
	"fmt"

	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

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
