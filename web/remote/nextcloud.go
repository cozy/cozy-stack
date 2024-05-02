package remote

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/nextcloud"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/webdav"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

func nextcloudGet(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.PUT, consts.Files); err != nil {
		return err
	}

	accountID := c.Param("account")
	nc, err := nextcloud.New(inst, accountID)
	if err != nil {
		return wrapNextcloudErrors(err)
	}

	path := c.Param("*")
	files, err := nc.ListFiles(path)
	if err != nil {
		return wrapNextcloudErrors(err)
	}
	return jsonapi.DataList(c, http.StatusOK, files, nil)
}

func nextcloudPut(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.PUT, consts.Files); err != nil {
		return err
	}

	accountID := c.Param("account")
	nc, err := nextcloud.New(inst, accountID)
	if err != nil {
		return wrapNextcloudErrors(err)
	}

	path := c.Param("*")
	if err := nc.Mkdir(path); err != nil {
		return wrapNextcloudErrors(err)
	}
	return c.JSON(http.StatusCreated, echo.Map{"ok": true})
}

func nextcloudRoutes(router *echo.Group) {
	group := router.Group("/nextcloud/:account")
	group.GET("/*", nextcloudGet)
	group.PUT("/*", nextcloudPut)
}

func wrapNextcloudErrors(err error) error {
	switch err {
	case nextcloud.ErrAccountNotFound:
		return jsonapi.NotFound(err)
	case nextcloud.ErrInvalidAccount:
		return jsonapi.BadRequest(err)
	case webdav.ErrInvalidAuth:
		return jsonapi.Unauthorized(err)
	case webdav.ErrAlreadyExist:
		return jsonapi.Conflict(err)
	case webdav.ErrParentNotFound:
		return jsonapi.NotFound(err)
	case webdav.ErrNotFound:
		return jsonapi.NotFound(err)
	case webdav.ErrInternalServerError:
		return jsonapi.InternalServerError(err)
	}
	return err
}
