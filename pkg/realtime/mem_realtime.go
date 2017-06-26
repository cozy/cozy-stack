package realtime

import (
	"errors"
	"sync"
	"sync/atomic"
)

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

func (h *memHub) Subscribe(domain, topicName string) EventChannel {
	topic := h.getOrCreate(domain, topicName)
	sub := &memSub{
		topic: topic,
		send:  make(chan *Event),
	}
	// Don't block on Subscribe
	go func() {
		topic.subscribe <- sub
	}()
	return sub
}

func (h *memHub) SubscribeLocalAll() EventChannel {
	return h.Subscribe("*", "*")
}

func (h *memHub) get(prefix, topicName string) *topic {
	h.RLock()
	defer h.RUnlock()
	return h.topics[h.topicKey(prefix, topicName)]
}

func (h *memHub) getOrCreate(prefix, topicName string) *topic {
	h.Lock()
	defer h.Unlock()
	key := h.topicKey(prefix, topicName)
	it, exists := h.topics[key]
	if !exists {
		it = newTopic(key)
		h.topics[key] = it
	}
	return it
}

func (h *memHub) remove(topic *topic) {
	h.Lock()
	defer h.Unlock()
	delete(h.topics, topic.key)
}

func (h *memHub) topicKey(domain, doctype string) string {
	return domain + ":" + doctype
}

type topic struct {
	key string

	// chans for subscribe/unsubscribe requests
	subscribe   chan *memSub
	unsubscribe chan *memSub
	broadcast   chan *Event

	// set of this topic subs, it should only be manipulated by the topic
	// loop goroutine
	subs map[*memSub]struct{}
}

func newTopic(key string) *topic {
	topic := &topic{
		key:         key,
		subscribe:   make(chan *memSub),
		unsubscribe: make(chan *memSub),
		broadcast:   make(chan *Event, 10),
		subs:        make(map[*memSub]struct{}),
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
				if !s.closed() {
					select {
					case s := <-t.unsubscribe:
						delete(t.subs, s)
					case s.send <- e:
					}
				}
			}
		}
	}
}

type memSub struct {
	topic *topic
	send  chan *Event
	c     uint32 // mark whether or not the sub is closed
}

func (s *memSub) Read() <-chan *Event {
	return s.send
}

func (s *memSub) closed() bool {
	return atomic.LoadUint32(&s.c) == 1
}

func (s *memSub) Close() error {
	if !atomic.CompareAndSwapUint32(&s.c, 0, 1) {
		return errors.New("closing a closed subscription")
	}
	// Don't block on Close
	go func() {
		s.topic.unsubscribe <- s
		close(s.send)
	}()
	return nil
}
