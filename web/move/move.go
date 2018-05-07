package move

import (
	"encoding/base64"
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/workers/move"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/permissions"
	"github.com/cozy/echo"
)

func exportHandler(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	if err := permissions.AllowWholeType(c, permissions.GET, consts.Exports); err != nil {
		return err
	}

	exportDoc, err := move.GetExportByID(inst, c.Param("export-id"))
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

	return move.ExportCopyData(c.Response(), inst, move.SystemArchiver(), exportMAC,
		c.QueryParam("cursor"))
}

func exportsListHandler(c echo.Context) error {
	inst := middlewares.GetInstance(c)

	if err := permissions.AllowWholeType(c, permissions.GET, consts.Exports); err != nil {
		return err
	}

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
	g.GET("/exports/:export-id", exportHandler)
	g.GET("/exports/data/:export-mac", exportDataHandler)
}
