package permissions

import (
	"errors"
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

// exports all constants from pkg/permissions to avoid double imports
var (
	ALL    = permissions.ALL
	GET    = permissions.GET
	PUT    = permissions.PUT
	POST   = permissions.POST
	PATCH  = permissions.PATCH
	DELETE = permissions.DELETE
)

// keyPicker choose the proper instance key depending on token audience
func keyPicker(i *instance.Instance) jwt.Keyfunc {
	return func(token *jwt.Token) (interface{}, error) {
		switch token.Claims.(*permissions.Claims).Audience {
		case permissions.AppAudience:
			return i.SessionSecret, nil
		case permissions.RefreshTokenAudience, permissions.AccessTokenAudience:
			return i.OAuthSecret, nil
		}
		return nil, permissions.ErrInvalidAudience
	}
}

const bearerAuthScheme = "Bearer "

// ErrNoToken is returned whe the request has no token
var ErrNoToken = errors.New("No token in request")

func getBearerToken(c echo.Context) string {
	header := c.Request().Header.Get(echo.HeaderAuthorization)
	if strings.HasPrefix(header, bearerAuthScheme) {
		return header[len(bearerAuthScheme):]
	}
	return ""
}

func getQueryToken(c echo.Context) string {
	return c.QueryParam("bearer_token")
}

// ContextPermissionSet is the key used in echo context to store permissions set
const ContextPermissionSet = "permissions_set"

// ContextClaims is the key used in echo context to store claims
// #nosec
const ContextClaims = "token_claims"

// Extractor extracts the permission set from the context
func Extractor(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {

		_, _, err := extract(c)
		if err != nil && err != ErrNoToken {
			// no token is provided, hopefully the next handler does not need one
			return err
		}

		return next(c)
	}
}

func extract(c echo.Context) (*permissions.Claims, *permissions.Set, error) {
	instance := middlewares.GetInstance(c)
	var claims permissions.Claims
	var err error

	if token := getBearerToken(c); token != "" {
		err = crypto.ParseJWT(token, keyPicker(instance), &claims)
	} else if token := getQueryToken(c); token != "" {
		err = crypto.ParseJWT(token, keyPicker(instance), &claims)
	} else {
		return nil, nil, ErrNoToken
	}

	if err != nil {
		return nil, nil, err
	}

	if claims.Issuer != instance.Domain {
		// invalid issuer in token
		return nil, nil, permissions.ErrInvalidToken
	}

	var pset permissions.Set
	if pset, err = claims.PermissionsSet(); err != nil {
		// invalid scope in token
		return nil, nil, permissions.ErrInvalidToken
	}

	c.Set(ContextClaims, claims)
	c.Set(ContextPermissionSet, pset)

	return &claims, &pset, err
}

func getPermission(c echo.Context) (*permissions.Set, error) {

	s, ok := c.Get(ContextPermissionSet).(permissions.Set)
	if ok {
		return &s, nil
	}

	_, set, err := extract(c)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusUnauthorized)
	}

	return set, nil
}

// Allow validate the validable object against the context permission set
func Allow(c echo.Context, v permissions.Verb, o permissions.Validable) error {
	pset, err := getPermission(c)
	if err != nil {
		return err
	}

	if !pset.Allow(v, o) {
		return echo.NewHTTPError(http.StatusForbidden)
	}
	return nil
}

// AllowTypeAndID validate a type & ID against the context permission set
func AllowTypeAndID(c echo.Context, v permissions.Verb, doctype, id string) error {
	pset, err := getPermission(c)
	if err != nil {
		return err
	}
	if !pset.AllowID(v, doctype, id) {
		return echo.NewHTTPError(http.StatusForbidden)
	}
	return nil
}

func displayPermissions(c echo.Context) error {
	set, err := getPermission(c)
	if err != nil {
		return err
	}
	return c.JSON(200, set)
}

// Routes sets the routing for the status service
func Routes(router *echo.Group) {
	// API Routes
	router.GET("/self", displayPermissions)
}
