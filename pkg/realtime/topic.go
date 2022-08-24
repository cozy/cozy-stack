package realtime

type filter struct {
	whole bool // true if the events for the whole doctype should be sent
	ids   []string
}

type toWatch struct {
	sub *EventsChan
	id  string
}

type topic struct {
	key string

	// chans for subscribe/unsubscribe requests
	subscribe   chan *toWatch
	unsubscribe chan *toWatch
	broadcast   chan *Event

	// set of this topic subs, it should only be manipulated by the topic
	// loop goroutine
	subs map[*EventsChan]filter
}

func newTopic(key string) *topic {
	topic := &topic{
		key:         key,
		subscribe:   make(chan *toWatch),
		unsubscribe: make(chan *toWatch),
		broadcast:   make(chan *Event, 10),
		subs:        make(map[*EventsChan]filter),
	}
	go topic.loop()
	return topic
}

func (t *topic) loop() {
	for {
		select {
		case w := <-t.unsubscribe:
			if w.id == "" {
				delete(t.subs, w.sub)
			} else if f, ok := t.subs[w.sub]; ok {
				ids := f.ids[:0]
				for _, id := range f.ids {
					if id != w.id {
						ids = append(ids, id)
					}
				}
				f.ids = ids
			}
		case w := <-t.subscribe:
			f := t.subs[w.sub]
			if w.id == "" {
				f.whole = true
			} else {
				f.ids = append(f.ids, w.id)
			}
			t.subs[w.sub] = f
		case e := <-t.broadcast:
			for s, f := range t.subs {
				ok := false
				if f.whole {
					ok = true
				} else {
					for _, id := range f.ids {
						if e.Doc.ID() == id {
							ok = true
							break
						}
					}
				}
				if ok {
					*s <- e
				}
			}
		}
	}
}
