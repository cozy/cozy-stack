package realtime

import (
	"encoding/json"
	"strings"

	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	redis "github.com/go-redis/redis"
)

const eventsRedisKey = "realtime:events"

type redisHub struct {
	c     redis.UniversalClient
	mem   *memHub
	local *topic
}

func newRedisHub(c redis.UniversalClient) *redisHub {
	local := newTopic("*")
	mem := newMemHub()
	hub := &redisHub{c, mem, local}
	go hub.start()
	return hub
}

type jsonEvent struct {
	Domain string                 `json:"domain"`
	Prefix string                 `json:"prefix"`
	Verb   string                 `json:"verb"`
	Doc    map[string]interface{} `json:"doc"`
	Old    map[string]interface{} `json:"old"`
}

// redefining couchdb.JSONDoc to avoid a dependency-cycle.
type jsonDoc struct {
	M    map[string]interface{}
	Type string
}

func (j jsonDoc) ID() string {
	id, _ := j.M["_id"].(string)
	return id
}

func (j jsonDoc) DocType() string {
	return j.Type
}

func toJSONDoc(m map[string]interface{}, doctype string) *jsonDoc {
	if m != nil {
		return &jsonDoc{M: m, Type: doctype}
	}
	return nil
}

func (h *redisHub) start() {
	sub := h.c.Subscribe(eventsRedisKey)
	log := logger.WithNamespace("realtime-redis")
	for msg := range sub.Channel() {
		parts := strings.SplitN(msg.Payload, ",", 2)
		if len(parts) < 2 {
			log.Warnf("Invalid payload: %s", msg.Payload)
			continue
		}
		var je jsonEvent
		doctype, payload := parts[0], parts[1]
		if err := json.Unmarshal([]byte(payload), &je); err != nil {
			log.Warnf("Error on start: %s", err)
			continue
		}
		if je.Doc == nil {
			continue
		}
		db := prefixer.NewPrefixer(je.Domain, je.Prefix)
		h.mem.Publish(db, je.Verb,
			toJSONDoc(je.Doc, doctype),
			toJSONDoc(je.Old, doctype))
	}
}

func (h *redisHub) GetTopic(db prefixer.Prefixer, doctype string) *topic {
	return nil
}

func (h *redisHub) Publish(db prefixer.Prefixer, verb string, doc, oldDoc Doc) {
	e := newEvent(db, verb, doc, oldDoc)
	h.local.broadcast <- e
	buf, err := json.Marshal(e)
	if err != nil {
		log := logger.WithNamespace("realtime-redis")
		log.Warnf("Error on publish: %s", err)
		return
	}
	h.c.Publish(eventsRedisKey, e.Doc.DocType()+","+string(buf))
}

func (h *redisHub) Subscriber(db prefixer.Prefixer) *DynamicSubscriber {
	return h.mem.Subscriber(db)
}

func (h *redisHub) SubscribeLocalAll() *DynamicSubscriber {
	ds := newDynamicSubscriber(nil, globalPrefixer)
	ds.addTopic(h.local, "")
	return ds
}
