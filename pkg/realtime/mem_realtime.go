package realtime

import (
	"sync"

	"github.com/cozy/cozy-stack/pkg/prefixer"
)

var globalPrefixer = prefixer.NewPrefixer(prefixer.GlobalCouchCluster, "", "*")

type memHub struct {
	sync.RWMutex
	topics map[string]*topic
}

func newMemHub() *memHub {
	return &memHub{topics: make(map[string]*topic)}
}

func (h *memHub) Publish(db prefixer.Prefixer, verb string, doc, oldDoc Doc) {
	e := newEvent(db, verb, doc, oldDoc)
	topic := h.get(e, doc.DocType())
	if topic != nil {
		topic.broadcast <- e
	}
	topic = h.get(globalPrefixer, "*")
	if topic != nil {
		topic.broadcast <- e
	}
}

func (h *memHub) Subscriber(db prefixer.Prefixer) *Subscriber {
	return newSubscriber(h, db)
}

func (h *memHub) SubscribeFirehose() *Subscriber {
	ds := newSubscriber(nil, globalPrefixer)
	t := h.GetTopic(globalPrefixer, "*")
	ds.addTopic(t, "")
	return ds
}

func (h *memHub) get(db prefixer.Prefixer, doctype string) *topic {
	h.RLock()
	defer h.RUnlock()
	return h.topics[h.topicKey(db, doctype)]
}

func (h *memHub) GetTopic(db prefixer.Prefixer, doctype string) *topic {
	h.Lock()
	defer h.Unlock()
	key := h.topicKey(db, doctype)
	it, exists := h.topics[key]
	if !exists {
		it = newTopic(key)
		h.topics[key] = it
	}
	return it
}

func (h *memHub) topicKey(db prefixer.Prefixer, doctype string) string {
	return db.DBPrefix() + ":" + doctype
}
