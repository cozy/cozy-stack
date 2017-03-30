package permissions

import (
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

const bearerAuthScheme = "Bearer "
const basicAuthScheme = "Basic "
const contextPermissionDoc = "permissions_doc"

// ErrNoToken is returned when the request has no token
var ErrNoToken = echo.NewHTTPError(http.StatusUnauthorized, "No token in request")

func hasRegisterToken(c echo.Context, i *instance.Instance) bool {
	hexToken := c.QueryParam("registerToken")
	expectedTok := i.RegisterToken

	if hexToken == "" || len(expectedTok) == 0 {
		return false
	}

	tok, err := hex.DecodeString(hexToken)
	if err != nil {
		return false
	}

	return subtle.ConstantTimeCompare(tok, expectedTok) == 1
}

func getRequestToken(c echo.Context) string {
	header := c.Request().Header.Get(echo.HeaderAuthorization)
	if strings.HasPrefix(header, bearerAuthScheme) {
		return header[len(bearerAuthScheme):]
	} else if strings.HasPrefix(header, basicAuthScheme) {
		_, pass, _ := c.Request().BasicAuth()
		return pass
	} else if tok := c.QueryParam("bearer_token"); tok != "" {
		return tok
	}
	return ""
}

func parseJWT(instance *instance.Instance, token string) (*permissions.Permission, error) {
	var claims permissions.Claims
	err := crypto.ParseJWT(token, func(token *jwt.Token) (interface{}, error) {
		return instance.PickKey(token.Claims.(*permissions.Claims).Audience)
	}, &claims)

	if err != nil {
		return nil, permissions.ErrInvalidToken
	}

	// check if the claim is valid
	if claims.Issuer != instance.Domain {
		return nil, permissions.ErrInvalidToken
	}

	if claims.Expired() {
		return nil, permissions.ErrExpiredToken
	}

	switch claims.Audience {
	case permissions.AccessTokenAudience:
		// An OAuth2 token is only valid if the client has not been revoked
		if _, err := oauth.FindClient(instance, claims.Subject); err != nil {
			return nil, permissions.ErrInvalidToken
		}

		return permissions.GetForOauth(&claims)

	case permissions.CLIAudience:
		// do not check client existence
		return permissions.GetForCLI(&claims)

	case permissions.AppAudience:
		pdoc, err := permissions.GetForApp(instance, claims.Subject)
		if err != nil {
			return nil, err
		}
		return pdoc, nil

	case permissions.ShareAudience:
		pdoc, err := permissions.GetForShareCode(instance, token)
		if err != nil {
			return nil, err
		}
		return pdoc, nil

	default:
		return nil, echo.NewHTTPError(http.StatusBadRequest, "Unrecognized token audience %v", claims.Audience)
	}
}

// extract permissions doc or set from the context
func extract(c echo.Context) (*permissions.Permission, error) {
	instance := middlewares.GetInstance(c)

	if hasRegisterToken(c, instance) {
		return permissions.GetForRegisterToken(), nil
	}

	var tok string
	if tok = getRequestToken(c); tok == "" {
		return nil, ErrNoToken
	}

	return parseJWT(instance, tok)

}

// GetPermission extracts the permission from the echo context and checks their validity
func GetPermission(c echo.Context) (*permissions.Permission, error) {

	pdoc, ok := c.Get(contextPermissionDoc).(*permissions.Permission)
	if ok && pdoc != nil {
		return pdoc, nil
	}

	pdoc, err := extract(c)
	if err != nil {
		return nil, err
	}

	c.Set(contextPermissionDoc, pdoc)

	return pdoc, nil
}
