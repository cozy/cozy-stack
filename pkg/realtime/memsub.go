package realtime

// Subscription to multiple hub channels.
type sub struct {
	topics []*topic
	closed chan bool
	send   chan *Event
}

func makeSub(topics []*topic) *sub {
	return &sub{
		topics: topics,
		closed: make(chan bool, 1),
		send:   make(chan *Event),
	}
}

// Read returns channel of receiver events.
func (s *sub) Read() <-chan *Event {
	return s.send
}

// Close removes subscriber from channel.
func (s *sub) Close() error {
	go func() {
		for _, t := range s.topics {
			t.unsubscribe <- s
		}
	}()
	go func() {
		s.closed <- true
		close(s.send)
	}()
	return nil
}

// CloseNotify returns channel to handle close event.
func (s *sub) CloseNotify() <-chan bool {
	return s.closed
}
