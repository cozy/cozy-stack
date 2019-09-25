package bitwarden

import (
	"encoding/base64"
	"net/http"
	"time"

	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

type transport struct {
	Transport string   `json:"transport"`
	Formats   []string `json:"transferFormats"`
}

// NegotiateHub is the handler for negociating between the server and the
// client which transport to use for bitwarden notifications. Currently,
// only websocket is supported.
func NegotiateHub(c echo.Context) error {
	if err := middlewares.AllowWholeType(c, permission.GET, consts.BitwardenCiphers); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}

	transports := []transport{
		// Bitwarden jslib supports only msgpack (Binary), not JSON (Text)
		{Transport: "WebSockets", Formats: []string{"Binary"}},
	}

	connID := crypto.GenerateRandomBytes(16)
	return c.JSON(http.StatusOK, echo.Map{
		"connectionId":        base64.URLEncoding.EncodeToString(connID),
		"availableTransports": transports,
	})
}

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer
	pongWait = 20 * time.Second
	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = 15 * time.Second
	// Maximum message size allowed from peer
	maxMessageSize = 1024
)

var upgrader = websocket.Upgrader{
	// Don't check the origin of the connexion
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// WebsocketHub is the websocket handler for the hub to send notifications in
// real-time for bitwarden stuff.
func WebsocketHub(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	token := c.QueryParam("access_token")
	pdoc, err := middlewares.ParseJWT(c, inst, token)
	if err != nil || !pdoc.Permissions.AllowWholeType(permission.GET, consts.BitwardenCiphers) {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
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

	ds := realtime.GetHub().Subscriber(inst)
	defer ds.Close()
	go readPump(ws, ds)

	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case e := <-ds.Channel:
			if err := ws.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return err
			}
			res := map[string]interface{}{
				"ID": e.Doc.ID(),
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

var initialResponse = []byte{0x7b, 0x7d, 0x1e} // {}<RS>

func readPump(ws *websocket.Conn, ds *realtime.DynamicSubscriber) {
	var msg struct {
		Protocol string `json:"protocol"`
		Version  int    `json:"version"`
	}
	if err := ws.ReadJSON(&msg); err != nil {
		if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
			logger.WithDomain(ds.DomainName()).WithField("nspace", "bitwarden").
				Infof("Read error: %s", err)
		}
		return
	}
	if msg.Protocol != "messagepack" || msg.Version != 1 {
		logger.WithDomain(ds.DomainName()).WithField("nspace", "bitwarden").
			Infof("Unexpected message: %v", msg)
		return
	}
	if err := ws.WriteMessage(websocket.BinaryMessage, initialResponse); err != nil {
		logger.WithDomain(ds.DomainName()).WithField("nspace", "bitwarden").
			Infof("Write error: %s", err)
		return
	}

	// Just send back the pings from the client
	for {
		msgType, p, err := ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
				logger.WithDomain(ds.DomainName()).WithField("nspace", "bitwarden").
					Infof("Read error: %s", err)
			}
			return
		}
		if err := ws.WriteMessage(msgType, p); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
				logger.WithDomain(ds.DomainName()).WithField("nspace", "bitwarden").
					Infof("Write error: %s", err)
				return
			}
		}
	}
}
