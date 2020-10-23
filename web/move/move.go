package move

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/move"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

func createExport(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	err := limits.CheckRateLimit(inst, limits.ExportType)
	if limits.IsLimitReachedOrExceeded(err) {
		return echo.NewHTTPError(http.StatusNotFound, "Not found")
	}
	if err := middlewares.AllowWholeType(c, permission.POST, consts.Exports); err != nil {
		return err
	}

	var exportOptions move.ExportOptions
	if _, err := jsonapi.Bind(c.Request().Body, &exportOptions); err != nil {
		return err
	}
	// The contextual domain is used to send a link on the correct domain when
	// the user is accessing their cozy from a backup URL.
	exportOptions.ContextualDomain = inst.ContextualDomain()

	msg, err := job.NewMessage(exportOptions)
	if err != nil {
		return err
	}
	_, err = job.System().PushJob(inst, &job.JobRequest{
		WorkerType: "export",
		Message:    msg,
	})
	if err != nil {
		return err
	}
	return c.NoContent(http.StatusCreated)
}

func exportHandler(c echo.Context) error {
	if err := middlewares.AllowWholeType(c, permission.GET, consts.Exports); err != nil {
		return err
	}

	mac, err := base64.URLEncoding.DecodeString(c.Param("export-mac"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err)
	}
	inst := middlewares.GetInstance(c)
	exportDoc, err := move.GetExport(inst, mac)
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

	mac, err := base64.URLEncoding.DecodeString(c.Param("export-mac"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err)
	}
	exportDoc, err := move.GetExport(inst, mac)
	if err != nil {
		return err
	}

	cursor, err := move.ParseCursor(exportDoc, c.QueryParam("cursor"))
	if err != nil {
		return err
	}

	from := inst.SubDomain(consts.SettingsSlug).String()
	middlewares.AppendCSPRule(c, "frame-ancestors", from)

	w := c.Response()
	w.Header().Set("Content-Type", "application/zip")
	filename := "My Cozy.zip"
	if len(exportDoc.PartsCursors) > 0 {
		filename = fmt.Sprintf("My Cozy - part%03d.zip", cursor.Number)
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.WriteHeader(http.StatusOK)

	archiver := move.SystemArchiver()
	return move.ExportCopyData(w, inst, exportDoc, archiver, cursor)
}

func precheckImport(c echo.Context) error {
	if err := middlewares.AllowWholeType(c, permission.POST, consts.Imports); err != nil {
		return err
	}

	var options move.ImportOptions
	if _, err := jsonapi.Bind(c.Request().Body, &options); err != nil {
		return err
	}

	inst := middlewares.GetInstance(c)
	if err := move.CheckImport(inst, options.SettingsURL); err != nil {
		return wrapError(err)
	}

	return c.NoContent(http.StatusNoContent)
}

func createImport(c echo.Context) error {
	if err := middlewares.AllowWholeType(c, permission.POST, consts.Imports); err != nil {
		return err
	}

	var options move.ImportOptions
	if _, err := jsonapi.Bind(c.Request().Body, &options); err != nil {
		return err
	}

	inst := middlewares.GetInstance(c)
	if err := move.ScheduleImport(inst, options); err != nil {
		return c.Render(http.StatusInternalServerError, "error.html", echo.Map{
			"CozyUI":      middlewares.CozyUI(inst),
			"ThemeCSS":    middlewares.ThemeCSS(inst),
			"Domain":      inst.ContextualDomain(),
			"ContextName": inst.ContextName,
			"Error":       err.Error(),
			"Favicon":     middlewares.Favicon(inst),
		})
	}

	to := inst.PageURL("/move/importing", nil)
	return c.Redirect(http.StatusSeeOther, to)
}

func waitImportHasFinished(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	return c.Render(http.StatusOK, "import.html", echo.Map{
		"CozyUI":      middlewares.CozyUI(inst),
		"ThemeCSS":    middlewares.ThemeCSS(inst),
		"Favicon":     middlewares.Favicon(inst),
		"Domain":      inst.ContextualDomain(),
		"ContextName": inst.ContextName,
		"Title":       inst.Translate("Import Title"),
	})
}

// Routes defines the routing layout for the /move module.
func Routes(g *echo.Group) {
	g.POST("/exports", createExport)
	g.GET("/exports/:export-mac", exportHandler)
	g.GET("/exports/data/:export-mac", exportDataHandler)

	g.POST("/imports/precheck", precheckImport)
	g.POST("/imports", createImport)

	g.GET("/importing", waitImportHasFinished)
}

func wrapError(err error) error {
	switch err {
	case move.ErrExportNotFound:
		return jsonapi.NotFound(err)
	case move.ErrNotEnoughSpace:
		return jsonapi.Errorf(http.StatusRequestEntityTooLarge, "%s", err)
	}
	return err
}
