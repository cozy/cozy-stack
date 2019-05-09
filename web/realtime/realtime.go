package realtime

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
	"github.com/gorilla/websocket"
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

type command struct {
	Method  string `json:"method"`
	Payload struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"payload"`
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

func forbidden(cmd *command) *wsError {
	return &wsError{
		Event: "error",
		Payload: wsErrorPayload{
			Status: "403 Forbidden",
			Code:   "forbidden",
			Title:  fmt.Sprintf("The application can't subscribe to %s", cmd.Payload.Type),
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

func missingType(cmd *command) *wsError {
	return &wsError{
		Event: "error",
		Payload: wsErrorPayload{
			Status: "404 Page Not Found",
			Code:   "page not found",
			Title:  "The type parameter is mandatory for SUBSCRIBE",
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

func readPump(ctx context.Context, c echo.Context, i *instance.Instance, ws *websocket.Conn,
	ds *realtime.DynamicSubscriber, errc chan *wsError, withAuthentication bool) {
	defer close(errc)

	var err error
	var pdoc *permission.Permission

	if withAuthentication {
		var auth map[string]string
		if err = ws.ReadJSON(&auth); err != nil {
			sendErr(ctx, errc, unknownMethod(auth["method"], auth))
			return
		}
		if strings.ToUpper(auth["method"]) != "AUTH" {
			sendErr(ctx, errc, unknownMethod(auth["method"], auth))
			return
		}
		if auth["payload"] == "" {
			sendErr(ctx, errc, unauthorized(auth))
			return
		}
		pdoc, err = middlewares.ParseJWT(c, i, auth["payload"])
		if err != nil {
			sendErr(ctx, errc, unauthorized(auth))
			return
		}
	}

	for {
		cmd := &command{}
		if err = ws.ReadJSON(cmd); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
				logger.WithDomain(ds.DomainName()).WithField("nspace", "realtime").Infof("Error: %s", err)
			}
			break
		}

		if strings.ToUpper(cmd.Method) != "SUBSCRIBE" {
			sendErr(ctx, errc, unknownMethod(cmd.Method, cmd))
			continue
		}
		if cmd.Payload.Type == "" {
			sendErr(ctx, errc, missingType(cmd))
			continue
		}
		// XXX: no permissions are required for io.cozy.sharings.initial-sync
		if withAuthentication &&
			cmd.Payload.Type != consts.SharingsInitialSync &&
			!pdoc.Permissions.AllowWholeType(permission.GET, cmd.Payload.Type) {
			sendErr(ctx, errc, forbidden(cmd))
			continue
		}

		if cmd.Payload.ID == "" {
			err = ds.Subscribe(cmd.Payload.Type)
		} else {
			err = ds.Watch(cmd.Payload.Type, cmd.Payload.ID)
		}
		if err != nil {
			logger.WithDomain(ds.DomainName()).WithField("nspace", "realtime").Warnf("Error: %s", err)
		}
	}
}

func ws(c echo.Context) error {
	var db prefixer.Prefixer

	// The realtime webservice can be plugged in a context without instance
	// fetching. For instance in the administration server. In such case, we do
	// not need authentication
	inst, withAuthentication := middlewares.GetInstanceSafe(c)
	if !withAuthentication {
		db = prefixer.GlobalPrefixer
	} else {
		db = inst
	}

	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	ws.SetReadLimit(maxMessageSize)
	if err = ws.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		return nil
	}
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(pongWait))
	})

	ds := realtime.GetHub().Subscriber(db)
	defer ds.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errc := make(chan *wsError)
	go readPump(ctx, c, inst, ws, ds, errc, withAuthentication)

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
		case e := <-ds.Channel:
			if err := ws.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return err
			}
			res := wsResponse{
				Event: e.Verb,
				Payload: wsResponsePayload{
					Type: e.Doc.DocType(),
					ID:   e.Doc.ID(),
					Doc:  e.Doc,
				},
			}
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

// Routes set the routing for the realtime service
func Routes(router *echo.Group) {
	router.GET("/", ws)
}
