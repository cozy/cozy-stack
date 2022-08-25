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
	h.RLock()
	defer h.RUnlock()

	e := newEvent(db, verb, doc, oldDoc)
	it := h.topics[topicKey(db, doc.DocType())]
	if it != nil {
		it.broadcast <- e
	}
	it = h.topics[topicKey(globalPrefixer, "*")]
	if it != nil {
		it.broadcast <- e
	}
}

func (h *memHub) Subscriber(db prefixer.Prefixer) *Subscriber {
	return newSubscriber(h, db)
}

func (h *memHub) SubscribeFirehose() *Subscriber {
	sub := newSubscriber(h, globalPrefixer)
	key := topicKey(sub, "*")
	h.subscribe(sub, key)
	return sub
}

func (h *memHub) subscribe(sub *Subscriber, key string) {
	h.Lock()
	defer h.Unlock()

	if sub.closed() {
		return
	}

	it, exists := h.topics[key]
	if !exists {
		it = newTopic()
		h.topics[key] = it
	}

	it.subs[&sub.Channel] = filter{whole: true}

	sub.addTopic(key)
}

func (h *memHub) unsubscribe(sub *Subscriber, key string) {
	h.Lock()
	defer h.Unlock()

	if sub.closed() {
		return
	}

	it, exists := h.topics[key]
	if !exists {
		return
	}

	delete(it.subs, &sub.Channel)
	if len(it.subs) == 0 {
		delete(h.topics, key)
		close(it.broadcast)
	}

	sub.removeTopic(key)
}

func (h *memHub) watch(sub *Subscriber, key, id string) {
	h.Lock()
	defer h.Unlock()

	if sub.closed() {
		return
	}

	it, exists := h.topics[key]
	if !exists {
		it = newTopic()
		h.topics[key] = it
	}

	f := it.subs[&sub.Channel]
	if f.whole {
		return
	}
	f.ids = append(f.ids, id)
	it.subs[&sub.Channel] = f

	sub.addTopic(key)
}

func (h *memHub) unwatch(sub *Subscriber, key, id string) {
	h.Lock()
	defer h.Unlock()

	if sub.closed() {
		return
	}

	it, exists := h.topics[key]
	if !exists {
		return
	}

	f := it.subs[&sub.Channel]
	if f.whole {
		return
	}
	ids := f.ids[:0]
	for i := range f.ids {
		if f.ids[i] != id {
			ids = append(ids, f.ids[i])
		}
	}
	if len(ids) > 0 {
		f.ids = ids
		it.subs[&sub.Channel] = f
		return
	}

	delete(it.subs, &sub.Channel)
	if len(it.subs) == 0 {
		delete(h.topics, key)
		close(it.broadcast)
	}

	sub.removeTopic(key)
}

func topicKey(db prefixer.Prefixer, doctype string) string {
	return db.DBPrefix() + ":" + doctype
}
