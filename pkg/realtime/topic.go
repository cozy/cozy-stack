package realtime

type filter struct {
	whole bool // true if the events for the whole doctype should be sent
	ids   []string
}

type toWatch struct {
	sub *Subscriber
	id  string // empty string means the whole doctype
}

type topic struct {
	broadcast   chan *Event            // input
	subs        map[*Subscriber]filter // output
	subscribe   chan *toWatch
	unsubscribe chan *toWatch
	running     chan bool
}

func newTopic() *topic {
	topic := &topic{
		broadcast:   make(chan *Event, 10),
		subs:        make(map[*Subscriber]filter),
		subscribe:   make(chan *toWatch),
		unsubscribe: make(chan *toWatch),
		running:     make(chan bool),
	}
	go topic.loop()
	return topic
}

func (t *topic) loop() {
	for {
		select {
		case e := <-t.broadcast:
			t.publish(e)
		case w := <-t.subscribe:
			t.doSubscribe(w)
		case w := <-t.unsubscribe:
			t.doUnsubscribe(w)
			if len(t.subs) == 0 {
				close(t.running)
				return
			}
			t.running <- true
		}
	}
}

func (t *topic) publish(e *Event) {
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
			select {
			case s.Channel <- e:
			case <-s.running: // the subscriber has been closed
			}
		}
	}
}

func (t *topic) doSubscribe(w *toWatch) {
	f := t.subs[w.sub]
	if w.id == "" {
		f.whole = true
	} else {
		f.ids = append(f.ids, w.id)
	}
	t.subs[w.sub] = f
}

func (t *topic) doUnsubscribe(w *toWatch) {
	if w.id == "" {
		delete(t.subs, w.sub)
	} else if f, ok := t.subs[w.sub]; ok {
		ids := f.ids[:0]
		for _, id := range f.ids {
			if id != w.id {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 && !f.whole {
			delete(t.subs, w.sub)
		} else {
			f.ids = ids
			t.subs[w.sub] = f
		}
	}
}
