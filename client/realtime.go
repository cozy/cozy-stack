package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync/atomic"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/gorilla/websocket"
)

// RealtimeOptions contains the options to create the realtime subscription
// channel.
type RealtimeOptions struct {
	DocTypes []string
}

// RealtimeChannel is used to create a realtime connection with the server. The
// Channel method can be used to retrieve a channel on which the realtime
// events can be received.
type RealtimeChannel struct {
	socket *websocket.Conn
	ch     chan *RealtimeServerMessage
	closed uint32
}

// RealtimeClientMessage is a struct containing the structure of the client
// messages sent to the server.
type RealtimeClientMessage struct {
	Method  string      `json:"method"`
	Payload interface{} `json:"payload"`
}

// RealtimeServerMessage is a struct containing the structure of the server
// messages received by the client.
type RealtimeServerMessage struct {
	Event   string                `json:"event"`
	Payload RealtimeServerPayload `json:"payload"`
}

// RealtimeServerPayload is the payload content of the RealtimeServerMessage.
type RealtimeServerPayload struct {
	// Response payload
	Type string          `json:"type"`
	ID   string          `json:"id"`
	Doc  json.RawMessage `json:"doc"`

	// Error payload
	Status string `json:"status"`
	Code   string `json:"code"`
	Title  string `json:"title"`
}

// RealtimeClient returns a new RealtimeChannel that instantiate a realtime
// connection with the client server.
func (c *Client) RealtimeClient(opts RealtimeOptions) (*RealtimeChannel, error) {
	var scheme string
	if c.Scheme == "https" {
		scheme = "wss"
	} else {
		scheme = "ws"
	}

	var err error
	var authorizer request.Authorizer
	if c.Authorizer != nil {
		authorizer = c.Authorizer
	} else {
		authorizer, err = c.Authenticate()
	}
	if err != nil {
		return nil, err
	}

	u := url.URL{
		Scheme: scheme,
		Host:   c.Domain,
		Path:   "/realtime/",
	}
	headers := make(http.Header)
	if authHeader := authorizer.AuthHeader(); authHeader != "" {
		headers.Add("Authorization", authHeader)
	}
	socket, _, err := websocket.DefaultDialer.Dial(u.String(), headers)
	if err != nil {
		return nil, err
	}

	realtimeToken := authorizer.RealtimeToken()
	if realtimeToken != "" {
		err = socket.WriteJSON(RealtimeClientMessage{
			Method:  "AUTH",
			Payload: authorizer.RealtimeToken(),
		})
		if err != nil {
			return nil, err
		}
	}

	for _, docType := range opts.DocTypes {
		err = socket.WriteJSON(RealtimeClientMessage{
			Method: "SUBSCRIBE",
			Payload: struct {
				Type string `json:"type"`
			}{Type: docType},
		})
		if err != nil {
			socket.Close()
			return nil, err
		}
	}

	channel := &RealtimeChannel{
		socket: socket,
		ch:     make(chan *RealtimeServerMessage),
	}

	go channel.pump()

	return channel, nil
}

// Channel returns the channe of reatime server messages received by the client
// from the server.
func (r *RealtimeChannel) Channel() <-chan *RealtimeServerMessage {
	return r.ch
}

func (r *RealtimeChannel) pump() (err error) {
	defer func() {
		if err != nil && err != io.EOF && atomic.LoadUint32(&r.closed) == 0 {
			r.ch <- &RealtimeServerMessage{
				Event:   "error",
				Payload: RealtimeServerPayload{Title: err.Error()},
			}
		}
	}()
	for {
		var msg RealtimeServerMessage
		if err = r.socket.ReadJSON(&msg); err != nil {
			return
		}
		r.ch <- &msg
	}
}

// Close will close the underlying connection of the realtime channel and close
// the channel of messages.
func (r *RealtimeChannel) Close() error {
	if atomic.CompareAndSwapUint32(&r.closed, 0, 1) {
		err := r.socket.Close()
		close(r.ch)
		return err
	}
	return nil
}
