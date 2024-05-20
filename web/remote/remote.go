// Package remote is the used for the /remote routes. They are intended for
// requesting data that is not in the Cozy itself, but in a remote place.
package remote

import (
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/remote"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

func allDoctypes(c echo.Context) error {
	if err := middlewares.AllowWholeType(c, permission.GET, consts.Doctypes); err != nil {
		return wrapRemoteErr(err)
	}

	inst := middlewares.GetInstance(c)
	doctypes, err := remote.ListDoctypes(inst)
	if err != nil {
		return wrapRemoteErr(err)
	}
	return c.JSON(http.StatusOK, doctypes)
}

func remoteGet(c echo.Context) error {
	doctype := c.Param("doctype")
	slug, err := allowWholeType(c, permission.GET, doctype)
	if err != nil {
		return wrapRemoteErr(err)
	}
	instance := middlewares.GetInstance(c)
	remote, err := remote.Find(instance, doctype)
	if err != nil {
		return wrapRemoteErr(err)
	}
	if remote.Verb != "GET" {
		return jsonapi.MethodNotAllowed("GET")
	}
	err = remote.ProxyTo(instance, c.Response(), c.Request(), slug)
	if err != nil {
		return wrapRemoteErr(err)
	}
	return nil
}

func remotePost(c echo.Context) error {
	doctype := c.Param("doctype")
	slug, err := allowWholeType(c, permission.POST, doctype)
	if err != nil {
		return wrapRemoteErr(err)
	}
	instance := middlewares.GetInstance(c)
	remote, err := remote.Find(instance, doctype)
	if err != nil {
		return wrapRemoteErr(err)
	}
	if remote.Verb != "POST" {
		return jsonapi.MethodNotAllowed("POST")
	}
	err = remote.ProxyTo(instance, c.Response(), c.Request(), slug)
	if err != nil {
		return wrapRemoteErr(err)
	}
	return nil
}

func remoteAsset(c echo.Context) error {
	_, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}
	return wrapRemoteErr(remote.
		ProxyRemoteAsset(c.Param("asset-name"), c.Response()))
}

// Routes set the routing for the remote service
func Routes(router *echo.Group) {
	router.GET("/_all_doctypes", allDoctypes)
	router.GET("/:doctype", remoteGet)
	router.POST("/:doctype", remotePost)
	router.GET("/assets/:asset-name", remoteAsset)

	nextcloudRoutes(router)
}

func wrapRemoteErr(err error) error {
	switch err {
	case remote.ErrNotFoundRemote:
		return jsonapi.NotFound(err)
	case remote.ErrInvalidRequest:
		return jsonapi.BadRequest(err)
	case remote.ErrRequestFailed:
		return jsonapi.BadGateway(err)
	case remote.ErrInvalidVariables:
		return jsonapi.BadRequest(err)
	case remote.ErrMissingVar:
		return jsonapi.BadRequest(err)
	case remote.ErrInvalidContentType:
		return jsonapi.BadGateway(err)
	case remote.ErrRemoteAssetNotFound:
		return jsonapi.NotFound(err)
	}
	return err
}

func allowWholeType(c echo.Context, v permission.Verb, doctype string) (string, error) {
	pdoc, err := middlewares.GetPermission(c)
	if err != nil {
		return "", err
	}
	if !pdoc.Permissions.AllowWholeType(v, doctype) {
		return "", middlewares.ErrForbidden
	}
	slug := ""
	if parts := strings.SplitN(pdoc.SourceID, "/", 2); len(parts) > 1 {
		slug = parts[1]
	}
	return slug, nil
}
