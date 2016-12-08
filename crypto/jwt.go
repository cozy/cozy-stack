package crypto

import jwt "gopkg.in/dgrijalva/jwt-go.v3"

// SigningMethod is the algorithm choosed for signing JWT.
// Currently, it is HMAC-SHA-512
var SigningMethod = jwt.SigningMethodHS512

// NewJWT creates a JWT token with the given claims,
// and signs it with the secret
func NewJWT(secret []byte, claims jwt.Claims) (string, error) {
	token := jwt.NewWithClaims(SigningMethod, claims)
	return token.SignedString(secret)
}
