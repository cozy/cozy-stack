package move

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/move"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gorilla/websocket"
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
	mac, err := base64.URLEncoding.DecodeString(c.Param("export-mac"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err)
	}
	inst := middlewares.GetInstance(c)
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

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10
)

var upgrader = websocket.Upgrader{
	// Don't check the origin of the connexion
	CheckOrigin:     func(r *http.Request) bool { return true },
	Subprotocols:    []string{"io.cozy.websocket"},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func wsDone(ws *websocket.Conn, inst *instance.Instance) {
	redirect := inst.PageURL("/auth/login", nil)
	_ = ws.SetWriteDeadline(time.Now().Add(writeWait))
	_ = ws.WriteJSON(echo.Map{"redirect": redirect})
}

func wsImporting(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	if move.ImportIsFinished(inst) {
		wsDone(ws, inst)
		return nil
	}

	if err = ws.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		return err
	}
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(pongWait))
	})

	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	ds := realtime.GetHub().Subscriber(inst)
	defer ds.Close()
	if err = ds.Subscribe(consts.Jobs); err != nil {
		return err
	}

	for {
		select {
		case e := <-ds.Channel:
			doc, ok := e.Doc.(permission.Fetcher)
			if !ok {
				continue
			}
			worker := doc.Fetch("worker")
			state := doc.Fetch("state")
			if len(worker) != 1 || worker[0] != "import" || len(state) != 1 {
				continue
			}
			if s := job.State(state[0]); s != job.Done && s != job.Errored {
				continue
			}
			wsDone(ws, inst)
			return nil
		case <-ticker.C:
			if err := ws.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return err
			}
			if err := ws.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				return err
			}
		}
	}
}

// Routes defines the routing layout for the /move module.
func Routes(g *echo.Group) {
	g.POST("/exports", createExport)
	g.GET("/exports/:export-mac", exportHandler)
	g.GET("/exports/data/:export-mac", exportDataHandler)

	g.POST("/imports/precheck", precheckImport)
	g.POST("/imports", createImport)

	g.GET("/importing", waitImportHasFinished)
	g.GET("/importing/realtime", wsImporting)
}

func wrapError(err error) error {
	switch err {
	case move.ErrExportNotFound:
		return jsonapi.PreconditionFailed("url", err)
	case move.ErrNotEnoughSpace:
		return jsonapi.Errorf(http.StatusRequestEntityTooLarge, "%s", err)
	}
	return err
}
