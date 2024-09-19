package rag

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/labstack/echo/v4"
)

type ChatPayload struct {
	ChatCompletionID string
	Query            string `json:"q"`
}

type ChatCompletion struct {
	DocID    string        `json:"_id"`
	DocRev   string        `json:"_rev,omitempty"`
	Messages []ChatMessage `json:"messages"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

const (
	HumanRole = "human"
	AIRole    = "ai"
)

func (c *ChatCompletion) ID() string        { return c.DocID }
func (c *ChatCompletion) Rev() string       { return c.DocRev }
func (c *ChatCompletion) DocType() string   { return consts.ChatCompletions }
func (c *ChatCompletion) SetID(id string)   { c.DocID = id }
func (c *ChatCompletion) SetRev(rev string) { c.DocRev = rev }
func (c *ChatCompletion) Clone() couchdb.Doc {
	cloned := *c
	cloned.Messages = make([]ChatMessage, len(c.Messages))
	copy(cloned.Messages, c.Messages)
	return &cloned
}
func (c *ChatCompletion) Included() []jsonapi.Object             { return nil }
func (c *ChatCompletion) Relationships() jsonapi.RelationshipMap { return nil }
func (c *ChatCompletion) Links() *jsonapi.LinksList              { return nil }

var _ jsonapi.Object = (*ChatCompletion)(nil)

type QueryMessage struct {
	Task  string `json:"task"`
	DocID string `json:"doc_id"`
}

func Chat(inst *instance.Instance, payload ChatPayload) (*ChatCompletion, error) {
	var chat ChatCompletion
	err := couchdb.GetDoc(inst, consts.ChatCompletions, payload.ChatCompletionID, &chat)
	if couchdb.IsNotFoundError(err) {
		chat.DocID = payload.ChatCompletionID
	} else if err != nil {
		return nil, err
	}
	msg := ChatMessage{Role: HumanRole, Content: payload.Query}
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
	var chat ChatCompletion
	err := couchdb.GetDoc(inst, consts.ChatCompletions, query.DocID, &chat)
	if err != nil {
		return err
	}
	msg := chat.Messages[len(chat.Messages)-1]
	payload := map[string]interface{}{
		"q": msg.Content,
	}

	res, err := callRAGQuery(inst, payload)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("POST status code: %d", res.StatusCode)
	}

	// TODO streaming
	completion, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	answer := ChatMessage{
		Role:    AIRole,
		Content: string(completion),
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
	u.Path = fmt.Sprintf("/query/%s", inst.Domain)
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
