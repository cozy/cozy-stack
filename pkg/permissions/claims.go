package permissions

import (
	"time"

	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

// #nosec
const (
	// AppAudience is the audience for JWT used by client-side apps
	AppAudience = "app"

	// KonnectorAudience is the audience for JWT used by konnectors
	KonnectorAudience = "konn"

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
var (
	defaultValidityDuration = 24 * time.Hour

	appTokenValidityDuration       = 24 * time.Hour
	konnectorTokenValidityDuration = 30 * time.Minute
	cliTokenValidityDuration       = 30 * time.Minute

	shareTokenValidityDuration        = 7 * 24 * time.Hour
	registrationTokenValidityDuration = 7 * 24 * time.Hour
	accessTokenValidityDuration       = 7 * 24 * time.Hour
	refreshTokenValidityDuration      = 24 * time.Hour
)

// Claims is used for JWT used in OAuth2 flow and applications token
type Claims struct {
	jwt.StandardClaims
	Scope     string `json:"scope,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// IssuedAtUTC returns a time.Time struct of the IssuedAt field in UTC
// location.
func (claims *Claims) IssuedAtUTC() time.Time {
	return time.Unix(claims.IssuedAt, 0).UTC()
}

// Expired returns true if a Claim is expired
func (claims *Claims) Expired() bool {
	var validityDuration time.Duration
	switch claims.Audience {
	case AppAudience:
		if claims.SessionID == "" {
			// an app token with no session association is used for services which
			// should have tokens that have the same properties as the konnector's
			// tokens
			validityDuration = konnectorTokenValidityDuration
		} else {
			validityDuration = appTokenValidityDuration
		}
	case KonnectorAudience:
		validityDuration = konnectorTokenValidityDuration
	case CLIAudience:
		validityDuration = cliTokenValidityDuration
	case ShareAudience:
		validityDuration = shareTokenValidityDuration
	case RegistrationTokenAudience:
		validityDuration = registrationTokenValidityDuration
	case AccessTokenAudience:
		validityDuration = accessTokenValidityDuration
	case RefreshTokenAudience:
		validityDuration = refreshTokenValidityDuration
	default:
		validityDuration = defaultValidityDuration
	}
	validUntil := claims.IssuedAtUTC().Add(validityDuration)
	return validUntil.Before(time.Now().UTC())
}
