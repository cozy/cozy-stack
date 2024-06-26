package remote

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/model/nextcloud"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/webdav"
	"github.com/cozy/cozy-stack/web/files"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
	"github.com/ncw/swift/v2"
)

func nextcloudGetTrash(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.GET, consts.Files); err != nil {
		return err
	}

	accountID := c.Param("account")
	nc, err := nextcloud.New(inst, accountID)
	if err != nil {
		return wrapNextcloudErrors(err)
	}

	path := c.Param("*")
	files, err := nc.ListTrash(path)
	if err != nil {
		return wrapNextcloudErrors(err)
	}
	return jsonapi.DataList(c, http.StatusOK, files, nil)
}

func nextcloudDeleteTrash(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.DELETE, consts.Files); err != nil {
		return err
	}

	accountID := c.Param("account")
	nc, err := nextcloud.New(inst, accountID)
	if err != nil {
		return wrapNextcloudErrors(err)
	}

	path := "/trash/" + c.Param("*")
	if err := nc.DeleteTrash(path); err != nil {
		return wrapNextcloudErrors(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func nextcloudEmptyTrash(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.DELETE, consts.Files); err != nil {
		return err
	}

	accountID := c.Param("account")
	nc, err := nextcloud.New(inst, accountID)
	if err != nil {
		return wrapNextcloudErrors(err)
	}

	if err := nc.EmptyTrash(); err != nil {
		return wrapNextcloudErrors(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func nextcloudGet(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.GET, consts.Files); err != nil {
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
	if c.QueryParam("Type") == "file" {
		return nextcloudUpload(c, nc, path)
	}

	if err := nc.Mkdir(path); err != nil {
		return wrapNextcloudErrors(err)
	}
	return c.JSON(http.StatusCreated, echo.Map{"ok": true})
}

func nextcloudUpload(c echo.Context, nc *nextcloud.NextCloud, path string) error {
	req := c.Request()
	mime := req.Header.Get(echo.HeaderContentType)
	if err := nc.Upload(path, mime, req.ContentLength, req.Body); err != nil {
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

func nextcloudMove(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.POST, consts.Files); err != nil {
		return err
	}

	accountID := c.Param("account")
	nc, err := nextcloud.New(inst, accountID)
	if err != nil {
		return wrapNextcloudErrors(err)
	}

	oldPath := c.Param("*")
	newPath := c.QueryParam("To")
	if newPath == "" {
		return jsonapi.BadRequest(errors.New("missing To parameter"))
	}

	if err := nc.Move(oldPath, newPath); err != nil {
		return wrapNextcloudErrors(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func nextcloudCopy(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.POST, consts.Files); err != nil {
		return err
	}

	accountID := c.Param("account")
	nc, err := nextcloud.New(inst, accountID)
	if err != nil {
		return wrapNextcloudErrors(err)
	}

	oldPath := c.Param("*")
	newPath := oldPath
	if p := c.QueryParam("Path"); p != "" {
		newPath = p
	} else if newName := c.QueryParam("Name"); newName != "" {
		newPath = filepath.Join(filepath.Dir(oldPath), newName)
	} else {
		ext := filepath.Ext(oldPath)
		base := strings.TrimSuffix(oldPath, ext)
		suffix := inst.Translate("File copy Suffix")
		newPath = fmt.Sprintf("%s (%s)%s", base, suffix, ext)
	}

	if err := nc.Copy(oldPath, newPath); err != nil {
		return wrapNextcloudErrors(err)
	}
	return c.JSON(http.StatusCreated, echo.Map{"ok": true})
}

func nextcloudDownstream(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.POST, consts.Files); err != nil {
		return err
	}

	accountID := c.Param("account")
	nc, err := nextcloud.New(inst, accountID)
	if err != nil {
		return wrapNextcloudErrors(err)
	}

	path := c.Param("*")
	to := c.QueryParam("To")
	if to == "" {
		return jsonapi.BadRequest(errors.New("missing To parameter"))
	}

	kind := nextcloud.MoveOperation
	if isCopy, _ := strconv.ParseBool(c.QueryParam("Copy")); isCopy {
		kind = nextcloud.CopyOperation
	}

	cozyMetadata, _ := files.CozyMetadataFromClaims(c, true)
	f, err := nc.Downstream(path, to, kind, cozyMetadata)
	if err != nil {
		return wrapNextcloudErrors(err)
	}
	obj := files.NewFile(f, inst)
	return jsonapi.Data(c, http.StatusCreated, obj, nil)
}

func nextcloudUpstream(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.POST, consts.Files); err != nil {
		return err
	}

	accountID := c.Param("account")
	nc, err := nextcloud.New(inst, accountID)
	if err != nil {
		return wrapNextcloudErrors(err)
	}

	path := c.Param("*")
	from := c.QueryParam("From")
	if from == "" {
		return jsonapi.BadRequest(errors.New("missing From parameter"))
	}

	kind := nextcloud.MoveOperation
	if isCopy, _ := strconv.ParseBool(c.QueryParam("Copy")); isCopy {
		kind = nextcloud.CopyOperation
	}

	if err := nc.Upstream(path, from, kind); err != nil {
		return wrapNextcloudErrors(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func nextcloudRestore(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.POST, consts.Files); err != nil {
		return err
	}

	accountID := c.Param("account")
	nc, err := nextcloud.New(inst, accountID)
	if err != nil {
		return wrapNextcloudErrors(err)
	}

	path := c.Param("*")
	if err := nc.Restore(path); err != nil {
		return wrapNextcloudErrors(err)
	}
	return c.NoContent(http.StatusNoContent)
}

func nextcloudRoutes(router *echo.Group) {
	group := router.Group("/nextcloud/:account")
	group.GET("/trash/*", nextcloudGetTrash)
	group.DELETE("/trash/*", nextcloudDeleteTrash)
	group.DELETE("/trash", nextcloudEmptyTrash)
	group.GET("/*", nextcloudGet)
	group.PUT("/*", nextcloudPut)
	group.DELETE("/*", nextcloudDelete)
	group.POST("/move/*", nextcloudMove)
	group.POST("/copy/*", nextcloudCopy)
	group.POST("/downstream/*", nextcloudDownstream)
	group.POST("/upstream/*", nextcloudUpstream)
	group.POST("/restore/*", nextcloudRestore)
}

func wrapNextcloudErrors(err error) error {
	switch err {
	case nextcloud.ErrAccountNotFound:
		return jsonapi.NotFound(err)
	case nextcloud.ErrInvalidAccount:
		return jsonapi.BadRequest(err)
	case webdav.ErrInvalidAuth:
		return jsonapi.Unauthorized(err)
	case webdav.ErrAlreadyExist, vfs.ErrConflict:
		return jsonapi.Conflict(err)
	case webdav.ErrParentNotFound:
		return jsonapi.NotFound(err)
	case webdav.ErrNotFound, os.ErrNotExist, swift.ObjectNotFound:
		return jsonapi.NotFound(err)
	case webdav.ErrInternalServerError:
		return jsonapi.InternalServerError(err)
	}
	return err
}
