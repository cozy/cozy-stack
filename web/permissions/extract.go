package permissions

import (
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

const bearerAuthScheme = "Bearer "
const basicAuthScheme = "Basic "
const contextPermissionDoc = "permissions_doc"

var errNoToken = echo.NewHTTPError(http.StatusUnauthorized, "No token in request")

// CheckRegisterToken returns true if the registerToken is set and match the
// one from the instance.
func CheckRegisterToken(c echo.Context, i *instance.Instance) bool {
	if len(i.RegisterToken) == 0 {
		return false
	}
	hexToken := c.QueryParam("registerToken")
	if hexToken == "" {
		return false
	}
	tok, err := hex.DecodeString(hexToken)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(tok, i.RegisterToken) == 1
}

// GetRequestToken retrieves the token from the incoming request.
func GetRequestToken(c echo.Context) string {
	req := c.Request()
	if header := req.Header.Get(echo.HeaderAuthorization); header != "" {
		if strings.HasPrefix(header, bearerAuthScheme) {
			return header[len(bearerAuthScheme):]
		}
		if strings.HasPrefix(header, basicAuthScheme) {
			_, pass, _ := req.BasicAuth()
			return pass
		}
	}
	return c.QueryParam("bearer_token")
}

// ParseJWT parses a JSON Web Token, and returns the associated permissions.
func ParseJWT(c echo.Context, instance *instance.Instance, token string) (*permissions.Permission, error) {
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

	// If claims contains a SessionID, we check that we are actually authorized
	// with the corresponding session.
	if claims.SessionID != "" {
		s, ok := middlewares.GetSession(c)
		if !ok || s.ID() != claims.SessionID {
			return nil, permissions.ErrInvalidToken
		}
	}

	switch claims.Audience {
	case permissions.AccessTokenAudience:
		// An OAuth2 token is only valid if the client has not been revoked
		c, err := oauth.FindClient(instance, claims.Subject)
		if err != nil {
			if couchdb.IsInternalServerError(err) {
				return nil, err
			}
			return nil, permissions.ErrInvalidToken
		}
		return permissions.GetForOauth(&claims, c)

	case permissions.CLIAudience:
		// do not check client existence
		return permissions.GetForCLI(&claims)

	case permissions.AppAudience:
		pdoc, err := permissions.GetForWebapp(instance, claims.Subject)
		if err != nil {
			return nil, err
		}
		return pdoc, nil

	case permissions.KonnectorAudience:
		pdoc, err := permissions.GetForKonnector(instance, claims.Subject)
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
		return nil, echo.NewHTTPError(http.StatusBadRequest, "Unrecognized token audience "+claims.Audience)
	}
}

// GetPermission extracts the permission from the echo context and checks their validity
func GetPermission(c echo.Context) (*permissions.Permission, error) {
	var err error

	pdoc, ok := c.Get(contextPermissionDoc).(*permissions.Permission)
	if ok && pdoc != nil {
		return pdoc, nil
	}

	inst := middlewares.GetInstance(c)
	if CheckRegisterToken(c, inst) {
		return permissions.GetForRegisterToken(), nil
	}

	tok := GetRequestToken(c)
	if tok == "" {
		return nil, errNoToken
	}

	pdoc, err = ParseJWT(c, inst, tok)
	if err != nil {
		return nil, err
	}

	c.Set(contextPermissionDoc, pdoc)
	return pdoc, nil
}
