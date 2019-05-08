package middlewares

import (
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/echo"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
)

const bearerAuthScheme = "Bearer "
const basicAuthScheme = "Basic "
const contextPermissionDoc = "permissions_doc"

// ErrForbidden is used to send a forbidden response when the request does not
// have the right permissions.
var ErrForbidden = echo.NewHTTPError(http.StatusForbidden)

// ErrMissingSource is used to send a bad request when the SourceURL is missing
// from the request
var ErrMissingSource = echo.NewHTTPError(http.StatusBadRequest, "No Source in request")

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

type linkedAppScope struct {
	Doctype string
	Slug    string
}

func parseLinkedAppScope(scope string) (*linkedAppScope, error) {
	if !strings.HasPrefix(scope, "@") {
		return nil, fmt.Errorf("Scope %s is not a linked-app", scope)
	}
	splitted := strings.Split(strings.TrimPrefix(scope, "@"), "/")

	return &linkedAppScope{
		Doctype: splitted[0],
		Slug:    splitted[1],
	}, nil
}

// GetForOauth create a non-persisted permissions doc from a oauth token scopes
func GetForOauth(instance *instance.Instance, claims *permissions.Claims, c interface{}) (*permissions.Permission, error) {
	var set permissions.Set
	linkedAppScope, err := parseLinkedAppScope(claims.Scope)

	if err == nil && linkedAppScope != nil {
		// Translate to a real scope
		at := consts.NewAppType(linkedAppScope.Doctype)
		manifest, err := app.GetBySlug(instance, linkedAppScope.Slug, at)
		if err != nil {
			return nil, err
		}
		set = manifest.Permissions()
	} else {
		set, err = permissions.UnmarshalScopeString(claims.Scope)
		if err != nil {
			return nil, err
		}
	}

	pdoc := &permissions.Permission{
		Type:        permissions.TypeOauth,
		Permissions: set,
		SourceID:    claims.Subject,
		Client:      c,
	}
	return pdoc, nil
}

// ParseJWT parses a JSON Web Token, and returns the associated permissions.
func ParseJWT(c echo.Context, instance *instance.Instance, token string) (*permissions.Permission, error) {
	var claims permissions.Claims
	var err error

	if isShortCode, _ := regexp.MatchString("^(\\w|\\d){12}$", token); isShortCode { // token is a shortcode
		token, err = permissions.GetTokenFromShortcode(instance, token)
		if err != nil {
			return nil, err
		}
	}

	err = crypto.ParseJWT(token, func(token *jwt.Token) (interface{}, error) {
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
		s, ok := GetSession(c)
		if !ok || s.ID() != claims.SessionID {
			return nil, permissions.ErrInvalidToken
		}
	}

	switch claims.Audience {
	case consts.AccessTokenAudience:
		// An OAuth2 token is only valid if the client has not been revoked
		c, err := oauth.FindClient(instance, claims.Subject)
		if err != nil {
			if couchdb.IsInternalServerError(err) {
				return nil, err
			}
			return nil, permissions.ErrInvalidToken
		}
		return GetForOauth(instance, &claims, c)

	case consts.CLIAudience:
		// do not check client existence
		return permissions.GetForCLI(&claims)

	case consts.AppAudience:
		pdoc, err := permissions.GetForWebapp(instance, claims.Subject)
		if err != nil {
			return nil, err
		}
		return pdoc, nil

	case consts.KonnectorAudience:
		pdoc, err := permissions.GetForKonnector(instance, claims.Subject)
		if err != nil {
			return nil, err
		}
		return pdoc, nil

	case consts.ShareAudience:
		pdoc, err := permissions.GetForShareCode(instance, token)
		if err != nil {
			return nil, err
		}

		// A share token is only valid if the user has not been revoked
		if pdoc.Type == permissions.TypeSharePreview {
			sharingID := strings.Split(pdoc.SourceID, "/")
			sharingDoc, err := sharing.FindSharing(instance, sharingID[1])
			if err != nil {
				return nil, err
			}

			member, err := sharingDoc.FindMemberBySharecode(instance, token)
			if err != nil {
				return nil, err
			}

			if member.Status == sharing.MemberStatusRevoked {
				return nil, permissions.ErrInvalidToken

			}
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

	inst := GetInstance(c)
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

// AllowWholeType validates that the context permission set can use a verb on
// the whold doctype
func AllowWholeType(c echo.Context, v permissions.Verb, doctype string) error {
	pdoc, err := GetPermission(c)
	if err != nil {
		return err
	}
	if !pdoc.Permissions.AllowWholeType(v, doctype) {
		return ErrForbidden
	}
	return nil
}

// Allow validates the validable object against the context permission set
func Allow(c echo.Context, v permissions.Verb, o permissions.Matcher) error {
	pdoc, err := GetPermission(c)
	if err != nil {
		return err
	}
	if !pdoc.Permissions.Allow(v, o) {
		return ErrForbidden
	}
	return nil
}

// AllowOnFields validates the validable object againt the context permission
// set and ensure the selector validates the given fields.
func AllowOnFields(c echo.Context, v permissions.Verb, o permissions.Matcher, fields ...string) error {
	pdoc, err := GetPermission(c)
	if err != nil {
		return err
	}
	if !pdoc.Permissions.AllowOnFields(v, o, fields...) {
		return ErrForbidden
	}
	return nil
}

// AllowTypeAndID validates a type & ID against the context permission set
func AllowTypeAndID(c echo.Context, v permissions.Verb, doctype, id string) error {
	pdoc, err := GetPermission(c)
	if err != nil {
		return err
	}
	if !pdoc.Permissions.AllowID(v, doctype, id) {
		return ErrForbidden
	}
	return nil
}

// AllowVFS validates a vfs.Matcher against the context permission set
func AllowVFS(c echo.Context, v permissions.Verb, o vfs.Matcher) error {
	instance := GetInstance(c)
	pdoc, err := GetPermission(c)
	if err != nil {
		return err
	}
	err = vfs.Allows(instance.VFS(), pdoc.Permissions, v, o)
	if err != nil {
		return ErrForbidden
	}
	return nil
}

// AllowInstallApp checks that the current context is tied to the store app,
// which is the only app authorized to install or update other apps.
// It also allow the cozy-stack apps commands to work (CLI).
func AllowInstallApp(c echo.Context, appType consts.AppType, sourceURL string, v permissions.Verb) error {
	pdoc, err := GetPermission(c)
	if err != nil {
		return err
	}

	var docType string
	switch appType {
	case consts.KonnectorType:
		docType = consts.Konnectors
	case consts.WebappType:
		docType = consts.Apps
	}

	if docType == "" {
		return fmt.Errorf("unknown application type %s", string(appType))
	}
	switch pdoc.Type {
	case permissions.TypeCLI:
		// OK
	case permissions.TypeWebapp, permissions.TypeKonnector:
		if pdoc.SourceID != consts.Apps+"/"+consts.StoreSlug {
			return ErrForbidden
		}
		if sourceURL == "" {
			return ErrMissingSource
		}
		// The store can only install apps and konnectors from the registry
		if !strings.HasPrefix(sourceURL, "registry://") {
			return ErrForbidden
		}
	default:
		return ErrForbidden
	}
	if !pdoc.Permissions.AllowWholeType(v, docType) {
		return ErrForbidden
	}
	return nil
}

// AllowForApp checks that the permissions is valid and comes from an
// application. If valid, the application's slug is returned.
func AllowForApp(c echo.Context, v permissions.Verb, o permissions.Matcher) (slug string, err error) {
	pdoc, err := GetPermission(c)
	if err != nil {
		return "", err
	}
	if pdoc.Type != permissions.TypeWebapp && pdoc.Type != permissions.TypeKonnector {
		return "", ErrForbidden
	}
	if !pdoc.Permissions.Allow(v, o) {
		return "", ErrForbidden
	}
	return pdoc.SourceID, nil
}

// GetSourceID returns the sourceID of the permissions associated with the
// given context.
func GetSourceID(c echo.Context) (slug string, err error) {
	pdoc, err := GetPermission(c)
	if err != nil {
		return "", err
	}
	return pdoc.SourceID, nil
}

// AllowLogout checks if the current permission allows logging out.
// all apps can trigger a logout.
func AllowLogout(c echo.Context) bool {
	pdoc, err := GetPermission(c)
	if err != nil {
		return false
	}
	return pdoc.Type == permissions.TypeWebapp
}
