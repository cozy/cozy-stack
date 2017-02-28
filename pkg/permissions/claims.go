package permissions

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/crypto"
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

	// TokenValidityDuration is the duration where a token is valid in seconds (1 week)
	TokenValidityDuration = int64(7 * 24 * time.Hour / time.Second)
)

// Claims is used for JWT used in OAuth2 flow and applications token
type Claims struct {
	jwt.StandardClaims
	Scope string `json:"scope,omitempty"`
}

// Expired returns true if a Claim is expired
func (claims *Claims) Expired() bool {
	now := crypto.Timestamp()
	validUntil := claims.IssuedAt + TokenValidityDuration
	return validUntil < now
}
