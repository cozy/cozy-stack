package permissions

import jwt "gopkg.in/dgrijalva/jwt-go.v3"

// #nosec
const (
	// ContextAudience is the audience for JWT used by apps
	ContextAudience = "context"

	// RegistrationTokenAudience is the audience field of JWT for registration tokens
	RegistrationTokenAudience = "registration"

	// AccessTokenAudience is the audience field of JWT for access tokens
	AccessTokenAudience = "access"

	// RefreshTokenAudience is the audience field of JWT for refresh tokens
	RefreshTokenAudience = "refresh"
)

// Claims is used for JWT used in OAuth2 flow and applications token
type Claims struct {
	jwt.StandardClaims
	Scope string `json:"scope,omitempty"`
}
