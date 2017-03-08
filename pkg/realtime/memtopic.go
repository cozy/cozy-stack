package realtime

type topic struct {
	hub *hub
	key string

	// chans for subscribe/unsubscribe requests
	subscribe   chan *sub
	unsubscribe chan *sub
	broadcast   chan *Event

	empty chan bool

	// set of this topic subs, it should only be manipulated by the topic
	// loop goroutine
	subs map[*sub]struct{}
}

func (t *topic) publish(e *Event) {
	go func() { t.broadcast <- e }()
}

func topicKey(instance, doctype string) string {
	return instance + ":" + doctype
}

func makeTopic(h *hub, key string) *topic {
	// subscribers should only be manipulated by the hub loop
	// it is a Map(type -> Set(subscriber))
	topic := &topic{
		hub:         h,
		key:         key,
		subscribe:   make(chan *sub),
		unsubscribe: make(chan *sub),
		broadcast:   make(chan *Event),
		empty:       make(chan bool),
		subs:        make(map[*sub]struct{}),
	}

	go topic.loop()
	return topic
}

func (t *topic) loop() {
	for {
		select {
		case e := <-t.broadcast:
			for s := range t.subs {
				s.send <- e
			}

		case s := <-t.subscribe:
			t.subs[s] = struct{}{}

		case s := <-t.unsubscribe:
			delete(t.subs, s)
			if len(t.subs) == 0 {
				t.hub.remove(t)
			}
		}
	}
}
