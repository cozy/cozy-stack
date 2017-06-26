package realtime

import (
	"net/http"
	"strings"

	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	// Don't check the origin of the connexion, the Authorization header is enough
	CheckOrigin:  func(r *http.Request) bool { return true },
	Subprotocols: []string{"io.cozy.websocket"},
}

type command struct {
	Method  string `json:"method"`
	Payload struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"payload"`
}

func ws(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	// TODO accept multiple subscribe commands
	// TODO filter by id
	// TODO what do we do with include_docs?
	var cmd command
	if err := ws.ReadJSON(&cmd); err != nil {
		// TODO log err
		return nil
	}

	if strings.ToUpper(cmd.Method) != "SUBSCRIBE" {
		// TODO send an error
		return nil
	}
	// TODO check permissions
	ec := realtime.GetHub().Subscribe(instance.Domain, cmd.Payload.Type)
	defer ec.Close()

	for {
		e := <-ec.Read()
		if err := ws.WriteJSON(e); err != nil {
			return nil
		}
	}
}

// Routes set the routing for the realtime service
func Routes(router *echo.Group) {
	router.GET("/", ws)
}
