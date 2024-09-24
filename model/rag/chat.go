package rag

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/metadata"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/gofrs/uuid/v5"
	"github.com/labstack/echo/v4"
)

type ChatPayload struct {
	ChatConversationID string
	Query              string `json:"q"`
}

type ChatConversation struct {
	DocID    string                 `json:"_id"`
	DocRev   string                 `json:"_rev,omitempty"`
	Messages []ChatMessage          `json:"messages"`
	Metadata *metadata.CozyMetadata `json:"cozyMetadata"`
}

type ChatMessage struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

const (
	UserRole      = "user"
	AssistantRole = "assistant"
)

// DocTypeVersion represents the doctype version. Each time this document
// structure is modified, update this value
const DocTypeVersion = "1"

func (c *ChatConversation) ID() string        { return c.DocID }
func (c *ChatConversation) Rev() string       { return c.DocRev }
func (c *ChatConversation) DocType() string   { return consts.ChatConversations }
func (c *ChatConversation) SetID(id string)   { c.DocID = id }
func (c *ChatConversation) SetRev(rev string) { c.DocRev = rev }
func (c *ChatConversation) Clone() couchdb.Doc {
	cloned := *c
	cloned.Messages = make([]ChatMessage, len(c.Messages))
	copy(cloned.Messages, c.Messages)
	return &cloned
}
func (c *ChatConversation) Included() []jsonapi.Object             { return nil }
func (c *ChatConversation) Relationships() jsonapi.RelationshipMap { return nil }
func (c *ChatConversation) Links() *jsonapi.LinksList              { return nil }

var _ jsonapi.Object = (*ChatConversation)(nil)

type QueryMessage struct {
	Task  string `json:"task"`
	DocID string `json:"doc_id"`
}

func Chat(inst *instance.Instance, payload ChatPayload) (*ChatConversation, error) {
	var chat ChatConversation
	err := couchdb.GetDoc(inst, consts.ChatConversations, payload.ChatConversationID, &chat)
	if couchdb.IsNotFoundError(err) {
		chat.DocID = payload.ChatConversationID
		md := metadata.New()
		md.DocTypeVersion = DocTypeVersion
		md.UpdatedAt = md.CreatedAt
		chat.Metadata = md
	} else if err != nil {
		return nil, err
	} else {
		chat.Metadata.UpdatedAt = time.Now().UTC()
	}
	uuidv7, _ := uuid.NewV7()
	msg := ChatMessage{
		ID:        uuidv7.String(),
		Role:      UserRole,
		Content:   payload.Query,
		CreatedAt: time.Now().UTC(),
	}
	chat.Messages = append(chat.Messages, msg)
	if chat.DocRev == "" {
		err = couchdb.CreateNamedDocWithDB(inst, &chat)
	} else {
		err = couchdb.UpdateDoc(inst, &chat)
	}
	if err != nil {
		return nil, err
	}
	query, err := job.NewMessage(&QueryMessage{
		Task:  "chat-completion",
		DocID: chat.DocID,
	})
	if err != nil {
		return nil, err
	}
	_, err = job.System().PushJob(inst, &job.JobRequest{
		WorkerType: "rag-query",
		Message:    query,
	})
	if err != nil {
		return nil, err
	}
	return &chat, nil
}

func Query(inst *instance.Instance, logger logger.Logger, query QueryMessage) error {
	var chat ChatConversation
	err := couchdb.GetDoc(inst, consts.ChatConversations, query.DocID, &chat)
	if err != nil {
		return err
	}
	payload := map[string]interface{}{
		"messages": chat.Messages,
		"stream":   true,
	}

	res, err := callRAGQuery(inst, payload)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("POST status code: %d", res.StatusCode)
	}

	msg := chat.Messages[len(chat.Messages)-1]
	var completion string
	err = foreachSSE(res.Body, func(event map[string]interface{}) {
		switch event["object"] {
		case "delta", "done":
			content, _ := event["content"].(string)
			completion += content
			delta := couchdb.JSONDoc{
				Type: consts.ChatEvents,
				M:    event,
			}
			delta.SetID(msg.ID)
			go realtime.GetHub().Publish(inst, realtime.EventCreate, &delta, nil)
		default:
			// We can ignore done events
		}
	})
	if err != nil {
		return err
	}

	uuidv7, _ := uuid.NewV7()
	answer := ChatMessage{
		ID:        uuidv7.String(),
		Role:      AssistantRole,
		Content:   completion,
		CreatedAt: time.Now().UTC(),
	}
	chat.Messages = append(chat.Messages, answer)
	return couchdb.UpdateDoc(inst, &chat)
}

func callRAGQuery(inst *instance.Instance, payload map[string]interface{}) (*http.Response, error) {
	ragServer := inst.RAGServer()
	if ragServer.URL == "" {
		return nil, errors.New("no RAG server configured")
	}
	u, err := url.Parse(ragServer.URL)
	if err != nil {
		return nil, err
	}
	u.Path = fmt.Sprintf("/chat/%s", inst.Domain)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", echo.MIMEApplicationJSON)
	return http.DefaultClient.Do(req)
}

func foreachSSE(r io.Reader, fn func(event map[string]interface{})) error {
	rb := bufio.NewReader(r)
	for {
		bs, err := rb.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if bytes.Equal(bs, []byte("\n")) {
			continue
		}
		if bytes.HasPrefix(bs, []byte(":")) {
			continue
		}
		parts := bytes.SplitN(bs, []byte(": "), 2)
		if len(parts) != 2 {
			return errors.New("invalid SSE response")
		}
		if string(parts[0]) != "data" {
			return errors.New("invalid SSE response")
		}
		var event map[string]interface{}
		if err := json.Unmarshal(bytes.TrimSpace(parts[1]), &event); err != nil {
			return err
		}
		fn(event)
	}
}
