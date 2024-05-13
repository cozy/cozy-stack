package remote

import (
	"io"
	"net/http"
	"path/filepath"

	"github.com/cozy/cozy-stack/model/nextcloud"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
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
	if c.QueryParam("Dl") == "1" {
		return nextcloudDownload(c, nc, path)
	}

	files, err := nc.ListFiles(path)
	if err != nil {
		return wrapNextcloudErrors(err)
	}
	return jsonapi.DataList(c, http.StatusOK, files, nil)
}

func nextcloudDownload(c echo.Context, nc *nextcloud.NextCloud, path string) error {
	f, err := nc.Download(path)
	if err != nil {
		return wrapNextcloudErrors(err)
	}
	defer f.Content.Close()

	w := c.Response()
	header := w.Header()
	filename := filepath.Base(path)
	disposition := vfs.ContentDisposition("attachment", filename)
	header.Set(echo.HeaderContentDisposition, disposition)
	header.Set(echo.HeaderContentType, f.Mime)
	if f.Length != "" {
		header.Set(echo.HeaderContentLength, f.Length)
	}
	if f.LastModified != "" {
		header.Set(echo.HeaderLastModified, f.LastModified)
	}
	if f.ETag != "" {
		header.Set("Etag", f.ETag)
	}
	if !config.GetConfig().CSPDisabled {
		middlewares.AppendCSPRule(c, "form-action", "'none'")
	}

	w.WriteHeader(http.StatusOK)
	_, err = io.Copy(w, f.Content)
	return err
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

func nextcloudDelete(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.DELETE, consts.Files); err != nil {
		return err
	}

	accountID := c.Param("account")
	nc, err := nextcloud.New(inst, accountID)
	if err != nil {
		return wrapNextcloudErrors(err)
	}

	path := c.Param("*")
	if err := nc.Delete(path); err != nil {
		return wrapNextcloudErrors(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func nextcloudRoutes(router *echo.Group) {
	group := router.Group("/nextcloud/:account")
	group.GET("/*", nextcloudGet)
	group.PUT("/*", nextcloudPut)
	group.DELETE("/*", nextcloudDelete)
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
