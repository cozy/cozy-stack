package permission

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
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
	case consts.AppAudience:
		if claims.SessionID == "" {
			// an app token with no session association is used for services which
			// should have tokens that have the same properties as the konnector's
			// tokens
			validityDuration = consts.KonnectorTokenValidityDuration
		} else {
			validityDuration = consts.AppTokenValidityDuration
		}

	case consts.KonnectorAudience:
		validityDuration = consts.KonnectorTokenValidityDuration

	case consts.CLIAudience:
		validityDuration = consts.CLITokenValidityDuration

	case consts.AccessTokenAudience:
		validityDuration = consts.AccessTokenValidityDuration

	// Share, RefreshToken and RegistrationToken never expire
	case consts.ShareAudience, consts.RegistrationTokenAudience, consts.RefreshTokenAudience:
		return false

	default:
		validityDuration = consts.DefaultValidityDuration
	}
	validUntil := claims.IssuedAtUTC().Add(validityDuration)
	return validUntil.Before(time.Now().UTC())
}
