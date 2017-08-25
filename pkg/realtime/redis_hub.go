package realtime

import (
	"encoding/json"
	"strings"

	"github.com/cozy/cozy-stack/pkg/logger"
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
	if j.M == nil {
		return json.Marshal(map[string]string{"_type": j.Type})
	}
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
	log := logger.WithNamespace("realtime-redis")
	for msg := range sub.Channel() {
		je := jsonEvent{}
		parts := strings.SplitN(msg.Payload, ",", 2)
		if len(parts) < 2 {
			log.Warnf("Invalid payload: %s", msg.Payload)
			continue
		}
		doctype := parts[0]
		buf := []byte(parts[1])
		if err := json.Unmarshal(buf, &je); err != nil {
			log.Warnf("Error on start: %s", err)
			continue
		}
		if je.Doc != nil {
			je.Doc.Type = doctype
		}
		if je.Old != nil {
			je.Old.Type = doctype
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

func (h *redisHub) GetTopic(domain, doctype string) *topic {
	return nil
}

func (h *redisHub) Publish(e *Event) {
	h.local.broadcast <- e
	buf, err := json.Marshal(e)
	if err != nil {
		log := logger.WithNamespace("realtime-redis")
		log.Warnf("Error on publish: %s", err)
		return
	}
	h.c.Publish(eventsRedisKey, e.Doc.DocType()+","+string(buf))
}

func (h *redisHub) Subscriber(domain string) *DynamicSubscriber {
	return h.mem.Subscriber(domain)
}

func (h *redisHub) SubscribeLocalAll() *DynamicSubscriber {
	ds := newDynamicSubscriber(nil, "")
	ds.addTopic(h.local, "")
	return ds
}
