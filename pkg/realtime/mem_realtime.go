package realtime

import (
	"errors"
	"sync"
	"sync/atomic"
)

var mainMemTopics = &memTopics{topics: make(map[string]*topic)}

type memTopics struct {
	sync.RWMutex
	topics map[string]*topic
}

func (h *memTopics) get(prefix, topicName string) *topic {
	h.RLock()
	defer h.RUnlock()
	return h.topics[h.topicKey(prefix, topicName)]
}

func (h *memTopics) getOrCreate(prefix, topicName string) *topic {
	h.Lock()
	defer h.Unlock()
	key := h.topicKey(prefix, topicName)
	it, exists := h.topics[key]
	if !exists {
		it = newTopic(h, key)
		h.topics[key] = it
	}
	return it
}

func (h *memTopics) remove(topic *topic) {
	h.Lock()
	defer h.Unlock()
	delete(h.topics, topic.key)
}

func (h *memTopics) topicKey(domain, doctype string) string {
	return domain + ":" + doctype
}

type memHub struct {
	prefix string
	topics *memTopics
}

func (h *memHub) Publish(e *Event) {
	topic := h.topics.get(h.prefix, e.Doc.DocType())
	if topic != nil {
		topic.broadcast <- e
	}
}

func (h *memHub) Subscribe(t string) EventChannel {
	topic := h.topics.getOrCreate(h.prefix, t)
	sub := &memSub{
		topic: topic,
		send:  make(chan *Event),
	}
	topic.subscribe <- sub
	return sub
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
	s.topic.unsubscribe <- s
	close(s.send)
	return nil
}

type topic struct {
	hub *memTopics
	key string

	// chans for subscribe/unsubscribe requests
	subscribe   chan *memSub
	unsubscribe chan *memSub
	broadcast   chan *Event

	// set of this topic subs, it should only be manipulated by the topic
	// loop goroutine
	subs map[*memSub]struct{}
}

func newTopic(hub *memTopics, key string) *topic {
	// subscribers should only be manipulated by the hub loop
	// it is a Map(type -> Set(subscriber))
	topic := &topic{
		hub:         hub,
		key:         key,
		subscribe:   make(chan *memSub, 1), // 1-sized buffer to be async
		unsubscribe: make(chan *memSub, 1), // 1-sized buffer to be async
		broadcast:   make(chan *Event, 10),
		subs:        make(map[*memSub]struct{}),
	}
	go topic.loop()
	return topic
}

func (t *topic) loop() {
	for {
		select {
		case e := <-t.broadcast:
			for s := range t.subs {
				if !s.closed() {
					s.send <- e
				}
			}
		case s := <-t.subscribe:
			t.subs[s] = struct{}{}
		case s := <-t.unsubscribe:
			delete(t.subs, s)
			if len(t.subs) == 0 {
				t.hub.remove(t)
				return
			}
		}
	}
}
