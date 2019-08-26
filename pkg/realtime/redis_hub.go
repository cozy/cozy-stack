package realtime

import (
	"encoding/json"
	"strings"

	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	redis "github.com/go-redis/redis/v7"
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

// JSONDoc is a map representing a simple json object that implements
// the couchdb.Doc interface.
//
// Note: we can't use the couchdb.JSONDoc as the couchdb package imports the
// realtime package, and it would create an import loop. And we cannot move the
// JSONDoc from couchdb here, as some of its methods use other functions from
// the couchdb package.
type JSONDoc struct {
	M    map[string]interface{}
	Type string
}

func (j JSONDoc) ID() string      { id, _ := j.M["_id"].(string); return id }
func (j JSONDoc) DocType() string { return j.Type }
func (j *JSONDoc) MarshalJSON() ([]byte, error) {
	m := map[string]interface{}{"_type": j.Type}
	if j.M != nil {
		for k, v := range j.M {
			m[k] = v
		}
	}
	return json.Marshal(m)
}

func toJSONDoc(d map[string]interface{}) *JSONDoc {
	if d == nil {
		return nil
	}
	doctype, _ := d["_type"].(string)
	delete(d, "_type")
	return &JSONDoc{d, doctype}
}

type jsonEvent struct {
	Domain string
	Prefix string
	Verb   string
	Doc    *JSONDoc
	Old    *JSONDoc
}

func (j *jsonEvent) UnmarshalJSON(buf []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(buf, &m); err != nil {
		return err
	}
	j.Domain, _ = m["domain"].(string)
	j.Prefix, _ = m["prefix"].(string)
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
		db := prefixer.NewPrefixer(je.Domain, je.Prefix)
		h.mem.Publish(db, je.Verb, je.Doc, je.Old)
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
