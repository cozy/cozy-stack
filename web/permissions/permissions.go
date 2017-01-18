package permissions

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

var readable = true
var none = false

var blackList = map[string]bool{
	consts.Sessions:  none,
	consts.Files:     readable,
	consts.Instances: readable,
}

var writable = false
var methodMap = map[string]bool{
	http.MethodGet:    readable,
	http.MethodPost:   writable,
	http.MethodPut:    writable,
	http.MethodPatch:  writable,
	http.MethodDelete: writable,
}

func checkBlackList(c echo.Context, method, doctype string) error {
	readabletype, inblacklist := blackList[doctype]
	requestIsRead := methodMap[c.Request().Method]

	if !inblacklist {
		return nil
	}

	if requestIsRead && readabletype {
		return nil
	}

	if requestIsRead && !readabletype {
		return &echo.HTTPError{
			Code:    http.StatusForbidden,
			Message: fmt.Sprintf("reserved doctype %s unreadable", doctype),
		}
	}

	return &echo.HTTPError{
		Code:    http.StatusForbidden,
		Message: fmt.Sprintf("reserved doctype %s unwritable", doctype),
	}

}

// CheckDoc returns an error if the context doesnt allows the operation
func CheckDoc(c echo.Context, method string, doc couchdb.Doc) error {
	return checkBlackList(c, method, doc.DocType())
}

// CheckByID returns an error if the context doesnt allows the operation
func CheckByID(c echo.Context, method, doctype, docid string) error {
	return checkBlackList(c, method, doctype)
}

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
		instance := middlewares.GetInstance(c)
		var claims permissions.Claims
		var err error

		if token := getBearerToken(c); token != "" {
			err = crypto.ParseJWT(token, keyPicker(instance), &claims)
		} else if token := getQueryToken(c); token != "" {
			err = crypto.ParseJWT(token, keyPicker(instance), &claims)
		} else {
			// no token is provided, hopefully the next handler does not need one
			return next(c)
		}

		if err != nil {
			return err
		}

		if claims.Issuer != instance.Domain {
			// invalid issuer in token
			return permissions.ErrInvalidToken
		}

		var pset permissions.Set
		if pset, err = claims.PermissionsSet(); err != nil {
			// invalid scope in token
			return permissions.ErrInvalidToken
		}

		c.Set(ContextClaims, claims)
		c.Set(ContextPermissionSet, pset)

		return next(c)
	}
}

func displayPermissions(c echo.Context) error {
	setInterface := c.Get(ContextPermissionSet)
	set, ok := setInterface.(permissions.Set)
	if !ok {
		return errors.New("no permission set in context")
	}
	return c.JSON(200, set)
}

// Routes sets the routing for the status service
func Routes(router *echo.Group) {
	// API Routes
	router.GET("/self", displayPermissions)
}
