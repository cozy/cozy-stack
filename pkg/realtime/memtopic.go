package realtime

type topic struct {
	hub *hub
	key string

	// chans for subscribe/unsubscribe requests
	subscribe   chan *sub
	unsubscribe chan *sub
	broadcast   chan *Event

	// set of this topic subs, it should only be manipulated by the topic
	// loop goroutine
	subs map[*sub]struct{}
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
		subscribe:   make(chan *sub, 1), // 1-sized buffer to be async
		unsubscribe: make(chan *sub, 1), // 1-sized buffer to be async
		broadcast:   make(chan *Event, 10),
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
