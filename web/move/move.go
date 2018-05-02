package move

import (
	"encoding/hex"
	"io"
	"net/http"

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
	exportID := c.Param("export-id")
	exportDoc, err := move.GetExport(inst, exportID)
	if err != nil {
		return err
	}

	return jsonapi.Data(c, http.StatusOK, exportDoc, nil)
}

func exportDataHandler(c echo.Context) error {
	if !middlewares.IsLoggedIn(c) {
		return echo.NewHTTPError(http.StatusUnauthorized)
	}

	inst := middlewares.GetInstance(c)
	exportID := c.Param("export-id")
	exportMAC, _ := hex.DecodeString(c.QueryParam("mac"))
	exportDoc, err := move.GetExport(inst, exportID)
	if err != nil {
		return err
	}

	archive, err := move.SystemArchiver().OpenArchive(inst, exportDoc, exportMAC)
	if err != nil {
		return err
	}
	defer archive.Close()

	c.Response().Header().Set("Content-Type", "application/tar+gzip")
	c.Response().Header().Set("Content-Disposition", "attachment; cozy-export.tar.gz")
	_, err = io.Copy(c.Response(), archive)
	return err
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

func Routes(g *echo.Group) {
	g.GET("/exports", exportsListHandler)
	g.GET("/exports/:export-id", exportHandler)
	g.GET("/exports/:export-id/data", exportDataHandler)
}
