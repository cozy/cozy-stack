package realtime

import (
	"sync"

	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

var globalPrefixer = prefixer.NewPrefixer(prefixer.GlobalCouchCluster, "", "*")

type memHub struct {
	sync.RWMutex
	topics        map[string]*topic
	bySubscribers map[*Subscriber][]string // the list of topic keys by subscriber
}

func newMemHub() *memHub {
	return &memHub{
		topics:        make(map[string]*topic),
		bySubscribers: make(map[*Subscriber][]string),
	}
}

func (h *memHub) Publish(db prefixer.Prefixer, verb string, doc, oldDoc Doc) {
	h.RLock()
	defer h.RUnlock()

	e := newEvent(db, verb, doc, oldDoc)
	key := topicKey(db, doc.DocType())
	it := h.topics[key]
	if it != nil {
		select {
		case it.broadcast <- e:
		case running := <-it.running:
			logger.WithNamespace("realtime").
				Warnf("unexpected state: publish with running=%v", running)
			if !running {
				delete(h.topics, key)
			}
		}
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
	go func() {
		defer h.Unlock()

		h.addTopic(sub, key)

		w := &toWatch{sub, ""}
		for {
			it, exists := h.topics[key]
			if !exists {
				it = newTopic()
				h.topics[key] = it
			}

			select {
			case it.subscribe <- w:
				return
			case running := <-it.running:
				logger.WithNamespace("realtime").
					Warnf("unexpected state: subscribe with running=%v", running)
				if !running {
					delete(h.topics, key)
				}
			}
		}
	}()
}

func (h *memHub) unsubscribe(sub *Subscriber, key string) {
	h.Lock()
	go func() {
		defer h.Unlock()

		it, exists := h.topics[key]
		if !exists {
			return
		}

		h.removeTopic(sub, key)

		w := &toWatch{sub, ""}
		select {
		case it.unsubscribe <- w:
			if running := <-it.running; !running {
				delete(h.topics, key)
			}
		case running := <-it.running:
			logger.WithNamespace("realtime").
				Warnf("unexpected state: unsubscribe with running=%v", running)
			if !running {
				delete(h.topics, key)
			}
		}
	}()
}

func (h *memHub) watch(sub *Subscriber, key, id string) {
	h.Lock()
	go func() {
		defer h.Unlock()

		h.addTopic(sub, key)

		w := &toWatch{sub, id}
		for {
			it, exists := h.topics[key]
			if !exists {
				it = newTopic()
				h.topics[key] = it
			}

			select {
			case it.subscribe <- w:
				return
			case running := <-it.running:
				logger.WithNamespace("realtime").
					Warnf("unexpected state: watch with running=%v", running)
				if !running {
					delete(h.topics, key)
				}
			}
		}
	}()
}

func (h *memHub) unwatch(sub *Subscriber, key, id string) {
	h.Lock()
	go func() {
		defer h.Unlock()

		it, exists := h.topics[key]
		if !exists {
			return
		}

		w := &toWatch{sub, id}
		select {
		case it.unsubscribe <- w:
			if running := <-it.running; !running {
				delete(h.topics, key)
			}
		case running := <-it.running:
			logger.WithNamespace("realtime").
				Warnf("unexpected state: unwatch with running=%v", running)
			if !running {
				delete(h.topics, key)
			}
		}
	}()
}

func (h *memHub) close(sub *Subscriber) {
	h.RLock()
	list := h.bySubscribers[sub]
	h.RUnlock()
	keys := make([]string, len(list))
	copy(keys, list)

	for _, key := range keys {
		h.unsubscribe(sub, key)
	}
}

func (h *memHub) addTopic(sub *Subscriber, key string) {
	list := h.bySubscribers[sub]
	for _, k := range list {
		if k == key {
			return
		}
	}
	list = append(list, key)
	h.bySubscribers[sub] = list
}

func (h *memHub) removeTopic(sub *Subscriber, key string) {
	list := h.bySubscribers[sub]
	kept := list[:0]
	for _, k := range list {
		if k != key {
			kept = append(kept, k)
		}
	}
	if len(kept) == 0 {
		delete(h.bySubscribers, sub)
	} else {
		h.bySubscribers[sub] = kept
	}
}

func topicKey(db prefixer.Prefixer, doctype string) string {
	return db.DBPrefix() + ":" + doctype
}
