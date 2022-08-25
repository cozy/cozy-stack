package realtime

type filter struct {
	whole bool // true if the events for the whole doctype should be sent
	ids   []string
}

type topic struct {
	broadcast chan *Event

	// set of this topic subs, it should only be manipulated by the topic
	// loop goroutine
	subs map[*EventsChan]filter
}

func newTopic() *topic {
	topic := &topic{
		broadcast: make(chan *Event, 10),
		subs:      make(map[*EventsChan]filter),
	}
	go topic.loop()
	return topic
}

func (t *topic) loop() {
	for e := range t.broadcast {
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
