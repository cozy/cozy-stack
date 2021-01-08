package move

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/move"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/pkg/mail"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/web/auth"
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
	exportOptions.MoveTo = nil
	exportOptions.TokenSource = ""

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

func blockForImport(c echo.Context) error {
	if err := middlewares.AllowWholeType(c, permission.POST, consts.Imports); err != nil {
		return err
	}

	inst := middlewares.GetInstance(c)
	if source := c.QueryParam("source"); source != "" {
		doc, err := inst.SettingsDocument()
		if err != nil {
			return err
		}
		doc.SetID(consts.InstanceSettingsID)
		doc.M["move_from"] = source
		if err := couchdb.UpdateDoc(inst, doc); err != nil {
			return err
		}
	}

	if err := lifecycle.Block(inst, instance.BlockedMoving.Code); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func waitImportHasFinished(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	template := "import.html"
	title := "Import Title"
	source := "?"
	if inst.BlockingReason == instance.BlockedMoving.Code {
		template = "move_in_progress.html"
		title = "Move in progress Title"
		doc, err := inst.SettingsDocument()
		if err == nil {
			if from, ok := doc.M["moved_from"].(string); ok {
				source = from
			}
		}
	}
	return c.Render(http.StatusOK, template, echo.Map{
		"CozyUI":      middlewares.CozyUI(inst),
		"ThemeCSS":    middlewares.ThemeCSS(inst),
		"Favicon":     middlewares.Favicon(inst),
		"Domain":      inst.ContextualDomain(),
		"ContextName": inst.ContextName,
		"Title":       inst.Translate(title),
		"Source":      source,
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

func getAuthorizeCode(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if !middlewares.IsLoggedIn(c) {
		u := inst.PageURL("/auth/login", url.Values{
			"redirect": {inst.FromURL(c.Request().URL)},
		})
		return c.Redirect(http.StatusSeeOther, u)
	}

	err := limits.CheckRateLimit(inst, limits.ExportType)
	if limits.IsLimitReachedOrExceeded(err) {
		return echo.NewHTTPError(http.StatusNotFound, "Not found")
	}

	u, err := url.Parse(c.QueryParam("redirect_uri"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "bad url: could not parse")
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return echo.NewHTTPError(http.StatusBadRequest, "bad url: bad scheme")
	}

	access, err := oauth.CreateAccessCode(inst, move.SourceClientID, consts.ExportsRequests)
	if err != nil {
		return err
	}

	vault := auth.HasVault(inst)
	used, quota, err := auth.DiskInfo(inst.VFS())
	if err != nil {
		return err
	}

	q := u.Query()
	q.Set("state", c.QueryParam("state"))
	q.Set("code", access.Code)
	q.Set("vault", strconv.FormatBool(vault))
	q.Set("used", used)
	if quota != "" {
		q.Set("quota", quota)
	}
	u.RawQuery = q.Encode()
	u.Fragment = ""
	location := u.String() + "#"
	return c.Redirect(http.StatusSeeOther, location)
}

func initializeMove(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if !middlewares.IsLoggedIn(c) {
		u := inst.PageURL("/auth/login", url.Values{
			"redirect": {inst.SubDomain(consts.SettingsSlug).String()},
		})
		return c.Redirect(http.StatusSeeOther, u)
	}

	err := limits.CheckRateLimit(inst, limits.ExportType)
	if limits.IsLimitReachedOrExceeded(err) {
		return echo.NewHTTPError(http.StatusNotFound, "Not found")
	}

	u, err := url.Parse(inst.MoveURL())
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "bad url: could not parse")
	}
	u.Path = "/initialize"

	vault := auth.HasVault(inst)
	used, quota, err := auth.DiskInfo(inst.VFS())
	if err != nil {
		return err
	}

	client, err := move.CreateRequestClient(inst)
	if err != nil {
		return err
	}
	access, err := oauth.CreateAccessCode(inst, client.ClientID, move.MoveScope)
	if err != nil {
		return err
	}

	q := u.Query()
	q.Set("client_id", client.ClientID)
	q.Set("client_secret", client.ClientSecret)
	q.Set("code", access.Code)
	q.Set("vault", strconv.FormatBool(vault))
	q.Set("used", used)
	if quota != "" {
		q.Set("quota", quota)
	}
	q.Set("cozy_url", inst.PageURL("/", nil))
	u.RawQuery = q.Encode()
	return c.Redirect(http.StatusTemporaryRedirect, u.String())
}

func requestMove(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	var request *move.Request
	params, err := c.FormParams()
	if err == nil {
		request, err = move.CreateRequest(inst, params)
	}
	if err != nil {
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Title":       instance.DefaultTemplateTitle,
			"CozyUI":      middlewares.CozyUI(inst),
			"ThemeCSS":    middlewares.ThemeCSS(inst),
			"Domain":      inst.ContextualDomain(),
			"ContextName": inst.ContextName,
			"ErrorTitle":  "Error Title",
			"Error":       err.Error(),
			"Favicon":     middlewares.Favicon(inst),
		})
	}

	publicName, _ := inst.PublicName()
	mail := mail.Options{
		Mode:         mail.ModeFromStack,
		TemplateName: "move_confirm",
		TemplateValues: map[string]interface{}{
			"ConfirmLink": request.Link,
			"PublicName":  publicName,
			"Source":      inst.ContextualDomain(),
			"Target":      request.TargetHost(),
		},
	}
	msg, err := job.NewMessage(&mail)
	if err != nil {
		return err
	}
	_, err = job.System().PushJob(inst, &job.JobRequest{
		WorkerType: "sendmail",
		Message:    msg,
	})
	if err != nil {
		return err
	}

	email, _ := inst.SettingsEMail()
	return c.Render(http.StatusOK, "move_confirm.html", echo.Map{
		"CozyUI":      middlewares.CozyUI(inst),
		"ThemeCSS":    middlewares.ThemeCSS(inst),
		"Favicon":     middlewares.Favicon(inst),
		"Domain":      inst.ContextualDomain(),
		"ContextName": inst.ContextName,
		"Title":       inst.Translate("Move Confirm Title"),
		"Email":       email,
	})
}

func startMove(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if !middlewares.IsLoggedIn(c) {
		return echo.NewHTTPError(http.StatusUnauthorized, "You must be authenticated")
	}

	request, err := move.StartMove(inst, c.QueryParam("secret"))
	if err != nil {
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Title":       instance.DefaultTemplateTitle,
			"CozyUI":      middlewares.CozyUI(inst),
			"ThemeCSS":    middlewares.ThemeCSS(inst),
			"Domain":      inst.ContextualDomain(),
			"ContextName": inst.ContextName,
			"ErrorTitle":  "Error Title",
			"Error":       err.Error(),
			"Favicon":     middlewares.Favicon(inst),
		})
	}

	return c.Redirect(http.StatusSeeOther, request.ImportingURL())
}

func finalizeMove(c echo.Context) error {
	if err := middlewares.AllowWholeType(c, permission.POST, consts.Imports); err != nil {
		return err
	}

	inst := middlewares.GetInstance(c)
	if err := move.Finalize(inst); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func abortMove(c echo.Context) error {
	if err := middlewares.AllowWholeType(c, permission.POST, consts.Imports); err != nil {
		return err
	}

	inst := middlewares.GetInstance(c)
	if err := lifecycle.Unblock(inst); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// Routes defines the routing layout for the /move module.
func Routes(g *echo.Group) {
	g.POST("/exports", createExport)
	g.GET("/exports/:export-mac", exportHandler)
	g.GET("/exports/data/:export-mac", exportDataHandler)

	g.POST("/imports/precheck", precheckImport)
	g.POST("/imports", createImport)

	g.POST("/importing", blockForImport)
	g.GET("/importing", waitImportHasFinished)
	g.GET("/importing/realtime", wsImporting)

	g.GET("/authorize", getAuthorizeCode)
	g.POST("/initialize", initializeMove)

	g.POST("/request", requestMove)
	g.GET("/go", startMove)
	g.POST("/finalize", finalizeMove)
	g.POST("/abort", abortMove)
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
