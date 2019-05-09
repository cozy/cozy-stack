package remote

import (
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/remote"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

func remoteGet(c echo.Context) error {
	doctype := c.Param("doctype")
	if err := middlewares.AllowWholeType(c, permission.GET, doctype); err != nil {
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
	err = remote.ProxyTo(doctype, instance, c.Response(), c.Request())
	if err != nil {
		return wrapRemoteErr(err)
	}
	return nil
}

func remotePost(c echo.Context) error {
	doctype := c.Param("doctype")
	if err := middlewares.AllowWholeType(c, permission.POST, doctype); err != nil {
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
	err = remote.ProxyTo(doctype, instance, c.Response(), c.Request())
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
	router.GET("/:doctype", remoteGet)
	router.POST("/:doctype", remotePost)
	router.GET("/assets/:asset-name", remoteAsset)
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
