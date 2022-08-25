package realtime

import (
	"errors"
	"sync/atomic"

	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// Subscriber is used to subscribe to several doctypes
type Subscriber struct {
	prefixer.Prefixer
	Channel EventsChan
	hub     Hub
	topics  map[string]struct{}
	c       uint32 // mark whether or not the sub is closed
}

// EventsChan is a chan of events
type EventsChan chan *Event

func newSubscriber(hub Hub, db prefixer.Prefixer) *Subscriber {
	return &Subscriber{
		Prefixer: db,
		Channel:  make(chan *Event, 10),
		hub:      hub,
		topics:   make(map[string]struct{}),
	}
}

// Subscribe adds a listener for events on a whole doctype
func (sub *Subscriber) Subscribe(doctype string) error {
	if sub.closed() || sub.hub == nil {
		return errors.New("Can't subscribe")
	}
	key := topicKey(sub, doctype)
	sub.hub.subscribe(sub, key)
	return nil
}

// Unsubscribe removes a listener for events on a whole doctype
func (sub *Subscriber) Unsubscribe(doctype string) error {
	if sub.closed() || sub.hub == nil {
		return errors.New("Can't subscribe")
	}
	key := topicKey(sub, doctype)
	sub.hub.unsubscribe(sub, key)
	return nil
}

// Watch adds a listener for events for a specific document (doctype+id)
func (sub *Subscriber) Watch(doctype, id string) error {
	if sub.closed() || sub.hub == nil {
		return errors.New("Can't subscribe")
	}
	key := topicKey(sub, doctype)
	sub.hub.watch(sub, key, id)
	return nil
}

// Unwatch removes a listener for events for a specific document (doctype+id)
func (sub *Subscriber) Unwatch(doctype, id string) error {
	if sub.closed() || sub.hub == nil {
		return errors.New("Can't subscribe")
	}
	key := topicKey(sub, doctype)
	sub.hub.unwatch(sub, key, id)
	return nil
}

func (sub *Subscriber) addTopic(key string) {
	sub.topics[key] = struct{}{}
}

func (sub *Subscriber) removeTopic(key string) {
	delete(sub.topics, key)
}

// closed returns true if it will no longer send events in its channel
func (sub *Subscriber) closed() bool {
	return atomic.LoadUint32(&sub.c) == 1
}

// Close closes the channel (async)
func (sub *Subscriber) Close() error {
	if !atomic.CompareAndSwapUint32(&sub.c, 0, 1) {
		return errors.New("closing a closed subscription")
	}

	for key := range sub.topics {
		sub.hub.unsubscribe(sub, key)
	}

	// Purge events, in a not-blocking way
	go func() {
		for range sub.Channel {
		}
		close(sub.Channel)
	}()

	return nil
}
