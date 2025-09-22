package sharings

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 1024
)

var upgrader = websocket.Upgrader{
	// Don't check the origin of the connexion, we check authorization later
	CheckOrigin:     func(r *http.Request) bool { return true },
	Subprotocols:    []string{"io.cozy.websocket"},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type wsResponsePayload struct {
	Type string      `json:"type"`
	ID   string      `json:"id"`
	Doc  interface{} `json:"doc,omitempty"`
}

type wsResponse struct {
	Event   string            `json:"event"`
	Payload wsResponsePayload `json:"payload"`
}

type wsErrorPayload struct {
	Status string      `json:"status"`
	Code   string      `json:"code"`
	Title  string      `json:"title"`
	Source interface{} `json:"source"`
}

type wsError struct {
	Event   string         `json:"event"`
	Payload wsErrorPayload `json:"payload"`
}

func unauthorized(cmd interface{}) *wsError {
	return &wsError{
		Event: "error",
		Payload: wsErrorPayload{
			Status: "401 Unauthorized",
			Code:   "unauthorized",
			Title:  "The authentication has failed",
			Source: cmd,
		},
	}
}

func unknownMethod(method string, cmd interface{}) *wsError {
	return &wsError{
		Event: "error",
		Payload: wsErrorPayload{
			Status: "405 Method Not Allowed",
			Code:   "method not allowed",
			Title:  fmt.Sprintf("The %s method is not supported", method),
			Source: cmd,
		},
	}
}

func sendErr(ctx context.Context, errc chan *wsError, e *wsError) {
	select {
	case errc <- e:
	case <-ctx.Done():
	}
}

func upgradeWs(c echo.Context) (*websocket.Conn, error) {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return nil, err
	}
	ws.SetReadLimit(maxMessageSize)
	if err = ws.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		return nil, errors.New("SetReadDeadline")
	}
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(pongWait))
	})
	return ws, nil
}

func getPermission(c echo.Context, i *instance.Instance, ws *websocket.Conn) (*permission.Permission, *wsError) {
	var auth map[string]string
	if err := ws.ReadJSON(&auth); err != nil {
		return nil, unknownMethod(auth["method"], auth)
	}
	if strings.ToUpper(auth["method"]) != "AUTH" {
		return nil, unknownMethod(auth["method"], auth)
	}
	if auth["payload"] == "" {
		return nil, unauthorized(auth)
	}
	pdoc, err := middlewares.ParseJWT(c, i, auth["payload"])
	if err != nil {
		return nil, unauthorized(auth)
	}
	return pdoc, nil
}

func wsWrite(ws *websocket.Conn, ch chan *wsResponse, errc chan *wsError) error {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case e, ok := <-errc:
			if !ok { // Websocket has been closed by the client
				return nil
			}
			if err := ws.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return nil
			}
			if err := ws.WriteJSON(e); err != nil {
				return nil
			}
		case res := <-ch:
			if err := ws.WriteJSON(res); err != nil {
				return nil
			}
		case <-ticker.C:
			if err := ws.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return err
			}
			if err := ws.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				return nil
			}
		}
	}
}

func filterMapEvents(ds *realtime.Subscriber, ch chan *wsResponse, inst *instance.Instance, s *sharing.Sharing) {
	var dir vfs.DirDoc
	if err := couchdb.GetDoc(inst, consts.Files, s.Rules[0].Values[0], &dir); err != nil {
		return
	}

	testPath := func(doc realtime.Doc) bool {
		if d, ok := doc.(*vfs.DirDoc); ok {
			return strings.HasPrefix(d.Fullpath, dir.Fullpath+"/")
		}
		if f, ok := doc.(*vfs.FileDoc); ok {
			if f.Trashed {
				return strings.HasPrefix(f.RestorePath, dir.Fullpath+"/")
			}
			p, err := f.Path(inst.VFS())
			if err != nil {
				return false
			}
			return strings.HasPrefix(p, dir.Fullpath+"/")
		}
		return false
	}

	match := func(e *realtime.Event) bool {
		if e.Doc.ID() == dir.DocID {
			return true
		}
		if e.OldDoc != nil && e.OldDoc.ID() == dir.DocID {
			return true
		}
		if testPath(e.Doc) {
			return true
		}
		if e.OldDoc != nil && testPath(e.OldDoc) {
			return true
		}
		return false
	}

	for e := range ds.Channel {
		if match(e) {
			ch <- &wsResponse{
				Event: e.Verb,
				Payload: wsResponsePayload{
					Type: e.Doc.DocType(),
					ID:   e.Doc.ID(),
					Doc:  e.Doc,
				},
			}
		}
	}
}

func wsOwner(c echo.Context, inst *instance.Instance, s *sharing.Sharing) error {
	ws, err := upgradeWs(c)
	if err != nil {
		return nil
	}
	defer ws.Close()

	ds := realtime.GetHub().Subscriber(inst)
	defer ds.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errc := make(chan *wsError)
	ch := make(chan *wsResponse)
	defer close(ch)

	go func() {
		defer close(errc)
		pdoc, wsErr := getPermission(c, inst, ws)
		if wsErr != nil {
			sendErr(ctx, errc, wsErr)
			return
		}
		for _, rule := range s.Rules {
			for _, value := range rule.Values {
				if !pdoc.Permissions.AllowID(permission.GET, rule.DocType, value) {
					sendErr(ctx, errc, unauthorized(pdoc))
					return
				}
			}
		}
		ds.Subscribe(consts.Files)
		filterMapEvents(ds, ch, inst, s)
	}()

	return wsWrite(ws, ch, errc)
}

// Ws is the API handler for realtime via a websocket connection.
func Ws(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	s, err := sharing.FindSharing(inst, c.Param("id"))
	if err != nil {
		return wrapErrors(err)
	}
	if !s.Drive {
		return jsonapi.NotFound(errors.New("not a drive"))
	}

	if s.Owner {
		return wsOwner(c, inst, s)
	}

	if len(s.Credentials) == 0 {
		return jsonapi.InternalServerError(errors.New("no credentials"))
	}
	token := s.Credentials[0].DriveToken
	u, err := url.Parse(s.Members[0].Instance)
	if err != nil {
		return jsonapi.InternalServerError(err)
	}

	// XXX Let's try to avoid one http request by cheating a bit. If the two
	// instances are on the same domain (same stack), we can watch directly
	// the real-time events. It helps for performances.
	if owner, err := lifecycle.GetInstance(u.Host); err == nil {
		return wsHijack(c, inst, owner, s)
	}
	return wsProxy(c, inst, s, token)
}

func wsHijack(c echo.Context, inst, owner *instance.Instance, s *sharing.Sharing) error {
	ws, err := upgradeWs(c)
	if err != nil {
		return nil
	}
	defer ws.Close()

	ds := realtime.GetHub().Subscriber(owner)
	defer ds.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errc := make(chan *wsError)
	ch := make(chan *wsResponse)
	defer close(ch)

	go func() {
		defer close(errc)
		pdoc, wsErr := getPermission(c, inst, ws)
		if wsErr != nil {
			sendErr(ctx, errc, wsErr)
			return
		}
		if !pdoc.Permissions.AllowWholeType(permission.GET, consts.Files) {
			sendErr(ctx, errc, unauthorized(pdoc))
			return
		}
		ds.Subscribe(consts.Files)
		filterMapEvents(ds, ch, owner, s)
	}()

	return wsWrite(ws, ch, errc)
}

func wsProxy(c echo.Context, inst *instance.Instance, s *sharing.Sharing, token string) error {
	ws, err := upgradeWs(c)
	if err != nil {
		return nil
	}
	defer ws.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errc := make(chan *wsError)
	ch := make(chan *wsResponse)
	defer close(ch)

	go func() {
		defer close(errc)
		pdoc, wsErr := getPermission(c, inst, ws)
		if wsErr != nil {
			sendErr(ctx, errc, wsErr)
			return
		}
		if !pdoc.Permissions.AllowWholeType(permission.GET, consts.Files) {
			sendErr(ctx, errc, unauthorized(pdoc))
			return
		}

		u, err := url.Parse(s.Members[0].Instance)
		if err != nil {
			return
		}
		var scheme string
		if u.Scheme == "https" {
			scheme = "wss"
		} else {
			scheme = "ws"
		}
		wsURL := url.URL{
			Scheme: scheme,
			Host:   u.Host,
			Path:   fmt.Sprintf("/sharings/drives/%s/realtime", s.SID),
		}

		ownerWS, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
		if err != nil {
			return
		}
		defer ownerWS.Close()

		authMsg := map[string]string{
			"method":  "AUTH",
			"payload": token,
		}
		if err := ownerWS.WriteJSON(authMsg); err != nil {
			return
		}

		for {
			var msg wsResponse
			if err := ownerWS.ReadJSON(&msg); err != nil {
				return
			}
			select {
			case ch <- &msg:
			case <-ctx.Done():
				return
			}
		}
	}()

	return wsWrite(ws, ch, errc)
}
