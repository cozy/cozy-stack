package move

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/workers/move"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/cozy/echo"
)

func exportHandler(c echo.Context) error {
	if err := permissions.AllowWholeType(c, permissions.GET, consts.Settings); err != nil {
		return err
	}

	inst := middlewares.GetInstance(c)
	exportMAC, err := base64.URLEncoding.DecodeString(c.Param("export-mac"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err)
	}
	exportDoc, err := move.GetExport(inst, exportMAC)
	if err != nil {
		return err
	}

	return jsonapi.Data(c, http.StatusOK, exportDoc, nil)
}

func exportDataHandler(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if !middlewares.IsLoggedIn(c) {
		u := inst.PageURL("/auth/login", url.Values{
			"redirect": {inst.FromURL(c.Request().URL)},
		})
		return c.Redirect(http.StatusSeeOther, u)
	}

	exportMAC, err := base64.URLEncoding.DecodeString(c.Param("export-mac"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err)
	}

	archive, size, err := move.ExportData(inst, move.SystemArchiver(), exportMAC)
	if err != nil {
		return err
	}
	defer archive.Close()

	c.Response().Header().Set("Content-Type", "application/tar+gzip")
	c.Response().Header().Set("Content-Disposition", "attachment; filename=cozy-export.tar.gz")
	c.Response().Header().Set("Content-Length", strconv.FormatInt(size, 10))
	_, err = io.Copy(c.Response(), archive)
	return err
}

func exportFilesHandler(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if !middlewares.IsLoggedIn(c) {
		u := inst.PageURL("/auth/login", url.Values{
			"redirect": {inst.FromURL(c.Request().URL)},
		})
		return c.Redirect(http.StatusSeeOther, u)
	}

	exportMAC, err := base64.URLEncoding.DecodeString(c.Param("export-mac"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err)
	}

	cursor := c.QueryParam("cursor")
	return move.ExportCopyFiles(c.Response(), inst, move.SystemArchiver(), exportMAC, cursor)
}

func exportsListHandler(c echo.Context) error {
	if err := permissions.AllowWholeType(c, permissions.GET, consts.Settings); err != nil {
		return err
	}

	inst := middlewares.GetInstance(c)
	exportDocs, err := move.GetExports(inst.Domain)
	if err != nil {
		return err
	}

	objs := make([]jsonapi.Object, len(exportDocs))
	for i, doc := range exportDocs {
		objs[i] = doc
	}

	return jsonapi.DataList(c, http.StatusOK, objs, nil)
}

// Routes defines the routing layout for the /move module.
func Routes(g *echo.Group) {
	g.GET("/exports", exportsListHandler)
	g.GET("/exports/:export-mac", exportHandler)
	g.GET("/exports/:export-mac/data", exportDataHandler)
	g.GET("/exports/:export-mac/files", exportFilesHandler)
}
