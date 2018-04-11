package permissions

import (
	"fmt"
	"net/http"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/vfs"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

var errForbidden = echo.NewHTTPError(http.StatusForbidden)

// AllowWholeType validates that the context permission set can use a verb on
// the whold doctype
func AllowWholeType(c echo.Context, v permissions.Verb, doctype string) error {
	pdoc, err := GetPermission(c)
	if err != nil {
		return err
	}
	if !pdoc.Permissions.AllowWholeType(v, doctype) {
		return errForbidden
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
		return errForbidden
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
		return errForbidden
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
		return errForbidden
	}
	return nil
}

// AllowVFS validates a vfs.Matcher against the context permission set
func AllowVFS(c echo.Context, v permissions.Verb, o vfs.Matcher) error {
	instance := middlewares.GetInstance(c)
	pdoc, err := GetPermission(c)
	if err != nil {
		return err
	}
	err = vfs.Allows(instance.VFS(), pdoc.Permissions, v, o)
	if err != nil {
		return errForbidden
	}
	return nil
}

// AllowInstallApp checks that the current context is tied to the store app,
// which is the only app authorized to install or update other apps.
// It also allow the cozy-stack apps commands to work (CLI).
func AllowInstallApp(c echo.Context, appType apps.AppType, v permissions.Verb) error {
	pdoc, err := GetPermission(c)
	if err != nil {
		return err
	}

	var docType string
	switch appType {
	case apps.Konnector:
		docType = consts.Konnectors
	case apps.Webapp:
		docType = consts.Apps
	}

	if docType == "" {
		return fmt.Errorf("unknown application type %s", string(appType))
	}
	switch pdoc.Type {
	case permissions.TypeCLI:
		// OK
	case permissions.TypeWebapp, permissions.TypeKonnector:
		if pdoc.SourceID != consts.Apps+"/"+consts.CollectSlug &&
			pdoc.SourceID != consts.Apps+"/"+consts.StoreSlug {
			return errForbidden
		}
	default:
		return errForbidden
	}
	if !pdoc.Permissions.AllowWholeType(v, docType) {
		return errForbidden
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
		return "", errForbidden
	}
	if !pdoc.Permissions.Allow(v, o) {
		return "", errForbidden
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

// AllowLogout checks if the current permission allows loging out.
// all apps can trigger a logout.
func AllowLogout(c echo.Context) bool {
	pdoc, err := GetPermission(c)
	if err != nil {
		return false
	}
	return pdoc.Type == permissions.TypeWebapp
}
