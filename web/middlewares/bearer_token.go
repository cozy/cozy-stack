package middlewares

import (
	"strings"

	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/labstack/echo"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

// ParseBearerAuth parses the Authorization HTTP header for a bearer token
func ParseBearerAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		header := c.Request().Header.Get("Authorization")
		parts := strings.Split(header, " ")
		if len(parts) == 2 && parts[0] == "Bearer" {
			instance := GetInstance(c)
			var claims permissions.Claims
			keyFunc := func(token *jwt.Token) (interface{}, error) {
				switch token.Claims.(*permissions.Claims).Audience {
				case permissions.AppAudience:
					return instance.SessionSecret, nil
				case permissions.RefreshTokenAudience, permissions.AccessTokenAudience:
					return instance.OAuthSecret, nil
				}
				return nil, permissions.ErrInvalidAudience
			}
			err := crypto.ParseJWT(parts[1], keyFunc, &claims)
			if err != nil {
				return err
			}
			if claims.Issuer != instance.Domain {
				return permissions.ErrInvalidToken
			}
			// TODO check validity
			c.Set("token_claims", claims)
		}
		return next(c)
	}
}
