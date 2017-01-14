package middlewares

import (
	"strings"

	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/labstack/echo"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

const bearerPrefix = "Bearer "

// ParseBearerAuth parses the Authorization HTTP header for a bearer token
func ParseBearerAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		auth := c.Request().Header.Get("Authorization")
		if !strings.HasPrefix(auth, bearerPrefix) {
			return next(c)
		}
		tokenString := auth[len(bearerPrefix):]
		instance := GetInstance(c)
		keyFunc := func(token *jwt.Token) (interface{}, error) {
			switch token.Claims.(*permissions.Claims).Audience {
			case permissions.AppAudience:
				return instance.SessionSecret, nil
			case permissions.RefreshTokenAudience, permissions.AccessTokenAudience:
				return instance.OAuthSecret, nil
			}
			return nil, permissions.ErrInvalidAudience
		}
		var claims permissions.Claims
		err := crypto.ParseJWT(tokenString, keyFunc, &claims)
		if err != nil {
			return err
		}
		if claims.Issuer != instance.Domain {
			return permissions.ErrInvalidToken
		}
		// TODO check validity
		c.Set("token_claims", claims)
		return next(c)
	}
}
