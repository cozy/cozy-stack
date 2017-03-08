package permissions

import (
	"time"

	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

// #nosec
const (
	// AppAudience is the audience for JWT used by client-side apps
	AppAudience = "app"

	// CliAudience is the audience for JWT used by command line interface
	CLIAudience = "cli"

	// ShareAudience is the audience field of JWT for access tokens
	ShareAudience = "share"

	// RegistrationTokenAudience is the audience field of JWT for registration tokens
	RegistrationTokenAudience = "registration"

	// AccessTokenAudience is the audience field of JWT for access tokens
	AccessTokenAudience = "access"

	// RefreshTokenAudience is the audience field of JWT for refresh tokens
	RefreshTokenAudience = "refresh"
)

// TokenValidityDuration is the duration where a token is valid in seconds (1 week)
var TokenValidityDuration = 7 * 24 * time.Hour

// Claims is used for JWT used in OAuth2 flow and applications token
type Claims struct {
	jwt.StandardClaims
	Scope string `json:"scope,omitempty"`
}

// IssuedAtUTC returns a time.Time struct of the IssuedAt field in UTC
// location.
func (claims *Claims) IssuedAtUTC() time.Time {
	return time.Unix(claims.IssuedAt, 0).UTC()
}

// Expired returns true if a Claim is expired
func (claims *Claims) Expired() bool {
	validUntil := claims.IssuedAtUTC().Add(TokenValidityDuration)
	return validUntil.Before(time.Now().UTC())
}
