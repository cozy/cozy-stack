package permissions

import jwt "gopkg.in/dgrijalva/jwt-go.v3"

// #nosec
const (
	// AppAudience is the audience for JWT used by client-side apps
	AppAudience = "app"

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

// PermissionsSet returns a set of Permissions parsed from this claims scope
func (c *Claims) PermissionsSet() (Set, error) {
	return UnmarshalScopeString(c.Scope)
}
