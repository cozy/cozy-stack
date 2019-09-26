package bitwarden

import (
	"encoding/base64"
	"net/http"
	"time"

	"github.com/cozy/cozy-stack/model/bitwarden"
	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/ugorji/go/codec"
)

type transport struct {
	Transport string   `json:"transport"`
	Formats   []string `json:"transferFormats"`
}

// NegotiateHub is the handler for negotiating between the server and the
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
	userID := pdoc.SourceID

	settings, err := settings.Get(inst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
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

	responses := make(chan []byte)
	ds := realtime.GetHub().Subscriber(inst)
	defer ds.Close()
	go readPump(ws, ds, responses)

	handle := new(codec.MsgpackHandle)
	handle.WriteExt = true
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case r := <-responses:
			if err := ws.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return err
			}
			if err := ws.WriteMessage(websocket.BinaryMessage, r); err != nil {
				logger.WithDomain(ds.DomainName()).WithField("nspace", "bitwarden").
					Infof("Write error: %s", err)
				return nil
			}
		case e := <-ds.Channel:
			if err := ws.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return err
			}
			notif := buildNotification(e, userID, settings)
			if notif == nil {
				continue
			}
			serialized, err := serializeNotification(handle, *notif)
			if err != nil {
				logger.WithDomain(ds.DomainName()).WithField("nspace", "bitwarden").
					Infof("Serialize error: %s", err)
				continue
			}
			if err := ws.WriteMessage(websocket.BinaryMessage, serialized); err != nil {
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

func readPump(ws *websocket.Conn, ds *realtime.DynamicSubscriber, responses chan []byte) {
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
	if err := ds.Subscribe(consts.BitwardenFolders); err != nil {
		logger.WithDomain(ds.DomainName()).WithField("nspace", "bitwarden").
			Infof("Subscribe error: %s", err)
		return
	}
	if err := ds.Subscribe(consts.BitwardenCiphers); err != nil {
		logger.WithDomain(ds.DomainName()).WithField("nspace", "bitwarden").
			Infof("Subscribe error: %s", err)
		return
	}
	responses <- initialResponse

	// Just send back the pings from the client
	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
				logger.WithDomain(ds.DomainName()).WithField("nspace", "bitwarden").
					Infof("Read error: %s", err)
			}
			return
		}
		responses <- msg
	}
}

type notificationResponse struct {
	ContextID string `codec:"ContextId"`
	Type      int
	Payload   map[string]interface{}
}

type notification []interface{}

// https://github.com/bitwarden/jslib/blob/master/src/enums/notificationType.ts
const (
	hubCipherUpdate = 0
	hubCipherCreate = 1
	// hubLoginDelete  = 2
	hubFolderDelete = 3
	// hubCiphers      = 4
	// hubVault        = 5
	// hubOrgKeys      = 6
	hubFolderCreate = 7
	hubFolderUpdate = 8
	hubCipherDelete = 9
	// hubSettings     = 10
	// hubLogOut       = 11
)

func buildNotification(e *realtime.Event, userID string, settings *settings.Settings) *notification {
	if e == nil || e.Doc == nil {
		return nil
	}

	doctype := e.Doc.DocType()
	t := -1
	var payload map[string]interface{}
	if doctype == consts.BitwardenFolders {
		payload = buildFolderPayload(e, userID)
		switch e.Verb {
		case realtime.EventCreate:
			t = hubFolderCreate
		case realtime.EventUpdate:
			t = hubFolderUpdate
		case realtime.EventDelete:
			t = hubFolderDelete
		}
	} else if doctype == consts.BitwardenCiphers {
		payload = buildCipherPayload(e, userID, settings)
		switch e.Verb {
		case realtime.EventCreate:
			t = hubCipherCreate
		case realtime.EventUpdate:
			t = hubCipherUpdate
		case realtime.EventDelete:
			t = hubCipherDelete
		}
	}
	if t < 0 {
		return nil
	}

	arg := notificationResponse{
		ContextID: "app_id",
		Type:      t,
		Payload:   payload,
	}
	msg := notification{
		1,                           // MessageType.Invocation
		[]interface{}{},             // Headers
		nil,                         // InvocationId
		"ReceiveMessage",            // Target
		[]notificationResponse{arg}, // Arguments
	}
	return &msg
}

func buildFolderPayload(e *realtime.Event, userID string) map[string]interface{} {
	var updatedAt interface{}
	var date string
	if doc, ok := e.Doc.(*couchdb.JSONDoc); ok {
		meta, _ := doc.M["cozyMetadata"].(map[string]interface{})
		date, _ = meta["updatedAt"].(string)
	} else if doc, ok := e.Doc.(*realtime.JSONDoc); ok {
		meta, _ := doc.M["cozyMetadata"].(map[string]interface{})
		date, _ = meta["updatedAt"].(string)
	} else if doc, ok := e.Doc.(*bitwarden.Folder); ok {
		if doc.Metadata != nil {
			updatedAt = doc.Metadata.UpdatedAt
		}
	}
	if date != "" {
		if t, err := time.Parse(time.RFC3339, date); err == nil {
			updatedAt = t
		}
	}
	if updatedAt == nil {
		updatedAt = time.Now()
	}
	return map[string]interface{}{
		"Id":           e.Doc.ID(),
		"UserId":       userID,
		"RevisionDate": updatedAt,
	}
}

func buildCipherPayload(e *realtime.Event, userID string, settings *settings.Settings) map[string]interface{} {
	var sharedWithCozy bool
	var updatedAt interface{}
	var date string
	if doc, ok := e.Doc.(*couchdb.JSONDoc); ok {
		sharedWithCozy, _ = doc.M["sharedWithCozy"].(bool)
		meta, _ := doc.M["cozyMetadata"].(map[string]interface{})
		date, _ = meta["updatedAt"].(string)
	} else if doc, ok := e.Doc.(*realtime.JSONDoc); ok {
		sharedWithCozy, _ = doc.M["sharedWithCozy"].(bool)
		meta, _ := doc.M["cozyMetadata"].(map[string]interface{})
		date, _ = meta["updatedAt"].(string)
	} else if doc, ok := e.Doc.(*bitwarden.Cipher); ok {
		sharedWithCozy = doc.SharedWithCozy
		if doc.Metadata != nil {
			updatedAt = doc.Metadata.UpdatedAt
		}
	}
	if date != "" {
		if t, err := time.Parse(time.RFC3339, date); err == nil {
			updatedAt = t
		}
	}
	if updatedAt == nil {
		updatedAt = time.Now()
	}
	var orgID, collIDs interface{}
	if sharedWithCozy {
		orgID = settings.OrganizationID
		collIDs = []string{settings.CollectionID}
	}
	return map[string]interface{}{
		"Id":             e.Doc.ID(),
		"UserId":         userID,
		"OrganizationId": orgID,
		"CollectionIds":  collIDs,
		"RevisionDate":   updatedAt,
	}
}

func serializeNotification(handle *codec.MsgpackHandle, notif notification) ([]byte, error) {
	// First serialize the notification to msgpack
	packed := make([]byte, 0, 256)
	encoder := codec.NewEncoderBytes(&packed, handle)
	if err := encoder.Encode(notif); err != nil {
		return nil, err
	}

	// Then, put it in a BinaryMessageFormat
	// https://github.com/aspnet/AspNetCore/blob/master/src/SignalR/clients/ts/signalr-protocol-msgpack/src/BinaryMessageFormat.ts
	size := uint(len(packed))
	lenBuf := make([]byte, 0, 8)
	for size > 0 {
		sizePart := size & 0x7f
		size >>= 7
		if size > 0 {
			sizePart |= 0x80
		}
		lenBuf = append(lenBuf, byte(sizePart))
	}
	buf := make([]byte, len(lenBuf)+len(packed))
	copy(buf[:len(lenBuf)], lenBuf)
	copy(buf[len(lenBuf):], packed)
	return buf, nil
}
