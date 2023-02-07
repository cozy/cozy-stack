package realtime

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	redis "github.com/redis/go-redis/v9"
)

const eventsRedisKey = "realtime:events"

type redisHub struct {
	c        redis.UniversalClient
	ctx      context.Context
	mem      *memHub
	firehose *topic
}

func newRedisHub(c redis.UniversalClient) *redisHub {
	ctx := context.Background()
	firehose := newTopic()
	mem := newMemHub()
	hub := &redisHub{c, ctx, mem, firehose}
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

// ID returns the ID of the document
func (j JSONDoc) ID() string { id, _ := j.M["_id"].(string); return id }

// DocType returns the DocType of the document
func (j JSONDoc) DocType() string { return j.Type }

// MarshalJSON is used for marshalling the document to JSON, with the doctype
// as _type.
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
	Cluster int
	Domain  string
	Prefix  string
	Verb    string
	Doc     *JSONDoc
	Old     *JSONDoc
}

func (j *jsonEvent) UnmarshalJSON(buf []byte) error {
	var m map[string]interface{}
	if err := json.Unmarshal(buf, &m); err != nil {
		return err
	}
	if cluster, ok := m["cluster"].(float64); ok {
		j.Cluster = int(cluster)
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
	sub := h.c.Subscribe(h.ctx, eventsRedisKey)
	log := logger.WithNamespace("realtime-redis")
	for msg := range sub.Channel() {
		parts := strings.SplitN(msg.Payload, ",", 2)
		if len(parts) < 2 {
			log.Warnf("Invalid payload: %s", msg.Payload)
			continue
		}
		// We clone the doctype to allow the GC to collect the payload even if
		// the jsonEvent is still in use.
		doctype := strings.Clone(parts[0])
		r := strings.NewReader(parts[1])
		je := jsonEvent{}
		if err := json.NewDecoder(r).Decode(&je); err != nil {
			log.Warnf("Error on start: %s", err)
			continue
		}
		if je.Doc != nil {
			je.Doc.Type = doctype
		}
		if je.Old != nil {
			je.Old.Type = doctype
		}
		db := prefixer.NewPrefixer(je.Cluster, je.Domain, je.Prefix)
		h.mem.Publish(db, je.Verb, je.Doc, je.Old)
	}
	logger.WithNamespace("realtime-redis").Infof("End of subscribe channel")
}

func (h *redisHub) Publish(db prefixer.Prefixer, verb string, doc, oldDoc Doc) {
	e := newEvent(db, verb, doc, oldDoc)
	h.firehose.broadcast <- e
	buf, err := json.Marshal(e)
	if err != nil {
		log := logger.WithNamespace("realtime-redis")
		log.Warnf("Error on publish: %s", err)
		return
	}
	h.c.Publish(h.ctx, eventsRedisKey, e.Doc.DocType()+","+string(buf))
}

func (h *redisHub) Subscriber(db prefixer.Prefixer) *Subscriber {
	return h.mem.Subscriber(db)
}

func (h *redisHub) SubscribeFirehose() *Subscriber {
	sub := newSubscriber(h, globalPrefixer)
	h.firehose.subscribe <- &toWatch{sub, ""}
	return sub
}

func (h *redisHub) subscribe(sub *Subscriber, key string) {
	panic("not reachable code")
}

func (h *redisHub) unsubscribe(sub *Subscriber, key string) {
	h.firehose.unsubscribe <- &toWatch{sub, ""}
	<-h.firehose.running
}

func (h *redisHub) watch(sub *Subscriber, key, id string) {
	panic("not reachable code")
}

func (h *redisHub) unwatch(sub *Subscriber, key, id string) {
	panic("not reachable code")
}

func (h *redisHub) close(sub *Subscriber) {
	h.unsubscribe(sub, "*")
}

var _ Doc = (*JSONDoc)(nil)
