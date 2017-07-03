package realtime

import "sync"

type memHub struct {
	sync.RWMutex
	topics map[string]*topic
}

func newMemHub() *memHub {
	return &memHub{topics: make(map[string]*topic)}
}

func (h *memHub) Publish(e *Event) {
	topic := h.get(e.Domain, e.Doc.DocType())
	if topic != nil {
		topic.broadcast <- e
	}
	topic = h.get("*", "*")
	if topic != nil {
		topic.broadcast <- e
	}
}

func (h *memHub) Subscriber(domain string) *DynamicSubscriber {
	return newDynamicSubscriber(h, domain)
}

func (h *memHub) SubscribeLocalAll() *DynamicSubscriber {
	ds := newDynamicSubscriber(nil, "")
	t := h.GetTopic("*", "*")
	ds.addTopic(t)
	return ds
}

func (h *memHub) get(domain, doctype string) *topic {
	h.RLock()
	defer h.RUnlock()
	return h.topics[h.topicKey(domain, doctype)]
}

func (h *memHub) GetTopic(domain, doctype string) *topic {
	h.Lock()
	defer h.Unlock()
	key := h.topicKey(domain, doctype)
	it, exists := h.topics[key]
	if !exists {
		it = newTopic(key)
		h.topics[key] = it
	}
	return it
}

func (h *memHub) topicKey(domain, doctype string) string {
	return domain + ":" + doctype
}

type topic struct {
	key string

	// chans for subscribe/unsubscribe requests
	subscribe   chan *MemSub
	unsubscribe chan *MemSub
	broadcast   chan *Event

	// set of this topic subs, it should only be manipulated by the topic
	// loop goroutine
	subs map[*MemSub]struct{}
}

func newTopic(key string) *topic {
	topic := &topic{
		key:         key,
		subscribe:   make(chan *MemSub),
		unsubscribe: make(chan *MemSub),
		broadcast:   make(chan *Event, 10),
		subs:        make(map[*MemSub]struct{}),
	}
	go topic.loop()
	return topic
}

func (t *topic) loop() {
	for {
		select {
		case s := <-t.unsubscribe:
			delete(t.subs, s)
		case s := <-t.subscribe:
			t.subs[s] = struct{}{}
		case e := <-t.broadcast:
			for s := range t.subs {
				*s <- e
			}
		}
	}
}
