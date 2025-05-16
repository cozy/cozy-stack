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
	ID        string        `json:"id"`
	Role      string        `json:"role"`
	Content   string        `json:"content"`
	Sources   []interface{} `json:"sources,omitempty"`
	CreatedAt time.Time     `json:"createdAt"`
}

const (
	UserRole      = "user"
	AssistantRole = "assistant"
	Temperature   = 0.3   // LLM parameter - Sampling temperature, lower is more deterministic, higher is more creative.
	TopP          = 1     // LLM parameter - Alternative to temperature, take the tokens with the top p probability.
	LogProbs      = false // LLM parameter - Whether to return log probabilities of the output tokens.
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

	type RAGMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	chat_history := make([]RAGMessage, 0, len(chat.Messages))
	for _, msg := range chat.Messages {
		chat_history = append(chat_history, RAGMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	payload := map[string]interface{}{
		"model":       fmt.Sprintf("ragondin-%s", inst.Domain),
		"messages":    chat_history,
		"stream":      true,
		"temperature": Temperature,
		"top_p":       TopP,
		"logprobs":    LogProbs,
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
	position := 0
	var completion string
	var sources []interface{}

	// Realtime messages are sent to the client during the response stream
	// When the stream is finished, the whole answer is saved in the CouchDB document
	err = foreachSSE(res.Body, func(event map[string]interface{}) {
		// See https://platform.openai.com/docs/api-reference/chat-streaming/streaming#chat-streaming
		if event["object"] == "chat.completion.chunk" {
			choices, ok := event["choices"].([]interface{})
			if !ok || len(choices) < 1 {
				return
			}
			choice := choices[0].(map[string]interface{}) // Only one choice is possible for now
			var doc map[string]interface{}

			if delta, ok := choice["delta"].(map[string]interface{}); ok {
				// The content is progressively reveived through a delta stream
				content, ok := delta["content"].(string)
				if !ok {
					return
				}
				doc = map[string]interface{}{
					"doc": map[string]interface{}{
						"_id":      msg.ID,
						"object":   "delta",
						"content":  content,
						"position": position,
					},
				}
				completion += content
				position++
			} else if reason, ok := choice["finish_reason"].(string); ok && reason != "" {
				// The response stream is finished
				doc = map[string]interface{}{
					"doc": map[string]interface{}{
						"_id":    msg.ID,
						"object": "done",
					},
				}
			}
			payload := couchdb.JSONDoc{
				Type: consts.ChatEvents,
				M:    doc,
			}
			payload.SetID(msg.ID)
			go realtime.GetHub().Publish(inst, realtime.EventCreate, &payload, nil)

			// Sources are sent in another realtime message
			if event["extra"] != nil {
				extra, ok := event["extra"].(map[string]interface{})
				if !ok {
					return
				}
				sourcesResp, ok := extra["sources"].([]interface{})
				if !ok {
					return
				}
				for _, s := range sourcesResp {
					obj, ok := s.(map[string]interface{})
					if !ok {
						continue
					}
					docID, ok := obj["doc_id"].(string)
					if !ok {
						continue
					}
					sources = append(sources, map[string]string{
						"id":      docID,
						"doctype": "io.cozy.files",
					})
				}

				sourceDoc := map[string]interface{}{
					"doc": map[string]interface{}{
						"_id":     msg.ID,
						"object":  "sources",
						"content": sources,
					},
				}
				sourcePayload := couchdb.JSONDoc{
					Type: consts.ChatEvents,
					M:    sourceDoc,
				}
				go realtime.GetHub().Publish(inst, realtime.EventCreate, &sourcePayload, nil)
			}
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
		Sources:   sources,
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

	u.Path = "v1/chat/completions"
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+ragServer.APIKey)
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
		if bytes.Equal(bs, []byte("\n")) || bytes.Equal(bs, []byte("\r\n")) {
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
			continue
		}
		data := bytes.TrimSpace(parts[1])
		if string(data) == "[DONE]" {
			break
		}
		var event map[string]interface{}
		if err := json.Unmarshal(data, &event); err != nil {
			return err
		}
		fn(event)
	}
	return nil
}
