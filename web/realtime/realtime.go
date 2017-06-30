package realtime

import (
	"fmt"
	"net/http"
	"strings"
	"time"

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
	// Don't check the origin of the connexion, the Authorization header is enough
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

func readPump(ws *websocket.Conn, ds *realtime.DynamicSubscriber) {
	defer ds.Close()
	for {
		var cmd command
		if err := ws.ReadJSON(&cmd); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				// TODO log err
			}
			break
		}

		if strings.ToUpper(cmd.Method) != "SUBSCRIBE" {
			// TODO send an error
			continue
		}

		// TODO check permissions
		// TODO filter by id
		// TODO what do we do with include_docs?
		ds.Subscribe(cmd.Payload.Type)
		fmt.Printf("Subscribed to %s\n", cmd.Payload.Type)
	}
}

func ws(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	ws.SetReadLimit(maxMessageSize)
	ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	ds := realtime.GetHub().Subscriber(instance.Domain)
	go readPump(ws, ds)

	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case e, ok := <-ds.Channel:
			if !ok {
				return nil
			}
			ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.WriteJSON(e); err != nil {
				return nil
			}
		case <-ticker.C:
			ws.SetWriteDeadline(time.Now().Add(writeWait))
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
