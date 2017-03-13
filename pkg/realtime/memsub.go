package realtime

import (
	"errors"
	"sync/atomic"
)

// Subscription to multiple hub channels.
type sub struct {
	topics []*topic
	send   chan *Event
	c      uint32 // mark whether or not the sub is closed
}

func makeSub(topics []*topic) *sub {
	return &sub{
		topics: topics,
		send:   make(chan *Event),
	}
}

// Read returns channel of receiver events.
func (s *sub) Read() <-chan *Event {
	return s.send
}

func (s *sub) closed() bool {
	return atomic.LoadUint32(&s.c) == 1
}

// Close removes subscriber from channel.
func (s *sub) Close() error {
	if !atomic.CompareAndSwapUint32(&s.c, 0, 1) {
		return errors.New("closing a closed subscription")
	}
	for _, t := range s.topics {
		t.unsubscribe <- s
	}
	close(s.send)
	return nil
}
