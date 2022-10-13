package realtime

import (
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// Subscriber is used to subscribe to several doctypes
type Subscriber struct {
	prefixer.Prefixer
	Channel EventsChan
	hub     Hub
	running chan struct{}
}

// EventsChan is a chan of events
type EventsChan chan *Event

func newSubscriber(hub Hub, db prefixer.Prefixer) *Subscriber {
	return &Subscriber{
		Prefixer: db,
		Channel:  make(chan *Event, 100),
		hub:      hub,
		running:  make(chan struct{}),
	}
}

// Subscribe adds a listener for events on a whole doctype
func (sub *Subscriber) Subscribe(doctype string) {
	if sub.hub == nil {
		return
	}
	key := topicKey(sub, doctype)
	go sub.hub.subscribe(sub, key)
}

// Unsubscribe removes a listener for events on a whole doctype
func (sub *Subscriber) Unsubscribe(doctype string) {
	if sub.hub == nil {
		return
	}
	key := topicKey(sub, doctype)
	go sub.hub.unsubscribe(sub, key)
}

// Watch adds a listener for events for a specific document (doctype+id)
func (sub *Subscriber) Watch(doctype, id string) {
	if sub.hub == nil {
		return
	}
	key := topicKey(sub, doctype)
	go sub.hub.watch(sub, key, id)
}

// Unwatch removes a listener for events for a specific document (doctype+id)
func (sub *Subscriber) Unwatch(doctype, id string) {
	if sub.hub == nil {
		return
	}
	key := topicKey(sub, doctype)
	go sub.hub.unwatch(sub, key, id)
}

// Close will unsubscribe to all topics and the subscriber should no longer be
// used after that.
func (sub *Subscriber) Close() {
	if sub.hub == nil {
		return
	}
	close(sub.running)
	go sub.hub.close(sub)
	sub.hub = nil
}
