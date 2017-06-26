package realtime

import (
	"encoding/json"
	"fmt"

	redis "github.com/go-redis/redis"
)

const eventsRedisKey = "realtime:events"

type redisHub struct {
	c     *redis.Client
	mem   *memHub
	local *topic
}

func newRedisHub(c *redis.Client) *redisHub {
	local := newTopic("*")
	mem := newMemHub()
	hub := &redisHub{c, mem, local}
	go hub.start()
	return hub
}

type jsonDoc struct {
	M    map[string]interface{}
	Type string
}

func (j jsonDoc) ID() string      { id, _ := j.M["_id"].(string); return id }
func (j jsonDoc) DocType() string { return j.Type }
func (j *jsonDoc) MarshalJSON() ([]byte, error) {
	j.M["_type"] = j.Type
	defer delete(j.M, "_type")
	return json.Marshal(j.M)
}

func toJSONDoc(d map[string]interface{}) *jsonDoc {
	if d == nil {
		return nil
	}
	doctype, _ := d["_type"].(string)
	delete(d, "_type")
	return &jsonDoc{d, doctype}
}

type jsonEvent struct {
	Domain string
	Verb   string
	Doc    *jsonDoc
	Old    *jsonDoc
}

func (j *jsonEvent) UnmarshalJSON(buf []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(buf, &m); err != nil {
		return err
	}
	j.Domain, _ = m["domain"].(string)
	j.Verb, _ = m["verb"].(string)
	if doc, ok := m["doc"].(map[string]interface{}); ok {
		j.Doc = toJSONDoc(doc)
	}
	if old, ok := m["old"].(map[string]interface{}); ok {
		j.Old = toJSONDoc(old)
	}
	return nil
}

func (h *redisHub) start() {
	sub := h.c.Subscribe(eventsRedisKey)
	for msg := range sub.Channel() {
		je := jsonEvent{}
		buf := []byte(msg.Payload)
		if err := json.Unmarshal(buf, &je); err != nil {
			fmt.Printf("Error on start: %s\n", err) // TODO log
			continue
		}
		e := &Event{
			Domain: je.Domain,
			Verb:   je.Verb,
			Doc:    je.Doc,
			OldDoc: je.Old,
		}
		h.mem.Publish(e)
	}
}

func (h *redisHub) Publish(e *Event) {
	h.local.broadcast <- e
	buf, err := json.Marshal(e)
	if err != nil {
		fmt.Printf("Error on publish: %s\n", err) // TODO log
		return
	}
	h.c.Publish(eventsRedisKey, string(buf))
}

func (h *redisHub) Subscribe(domain, topicName string) EventChannel {
	return h.mem.Subscribe(domain, topicName)
}

func (h *redisHub) SubscribeLocalAll() EventChannel {
	sub := &memSub{
		topic: h.local,
		send:  make(chan *Event),
	}
	// Don't block on Subscribe
	go func() {
		h.local.subscribe <- sub
	}()
	return sub
}
