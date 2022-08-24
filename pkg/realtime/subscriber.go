package realtime

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// Subscriber is used to subscribe to several doctypes
type Subscriber struct {
	prefixer.Prefixer
	Channel EventsChan
	hub     Hub
	topics  []*topic
	c       uint32 // mark whether or not the sub is closed
}

// EventsChan is a chan of events
type EventsChan chan *Event

func newSubscriber(hub Hub, db prefixer.Prefixer) *Subscriber {
	return &Subscriber{
		Prefixer: db,
		Channel:  make(chan *Event, 10),
		hub:      hub,
	}
}

// Subscribe adds a listener for events on a whole doctype
func (ds *Subscriber) Subscribe(doctype string) error {
	if ds.closed() || ds.hub == nil {
		return errors.New("Can't subscribe")
	}
	t := ds.hub.GetTopic(ds, doctype)
	ds.addTopic(t, "")
	return nil
}

// Unsubscribe removes a listener for events on a whole doctype
func (ds *Subscriber) Unsubscribe(doctype string) error {
	if ds.closed() || ds.hub == nil {
		return errors.New("Can't unsubscribe")
	}
	t := ds.hub.GetTopic(ds, doctype)
	ds.removeTopic(t, "")
	return nil
}

// Watch adds a listener for events for a specific document (doctype+id)
func (ds *Subscriber) Watch(doctype, id string) error {
	if ds.closed() || ds.hub == nil {
		return errors.New("Can't subscribe")
	}
	t := ds.hub.GetTopic(ds, doctype)
	ds.addTopic(t, id)
	return nil
}

// Unwatch removes a listener for events for a specific document (doctype+id)
func (ds *Subscriber) Unwatch(doctype, id string) error {
	if ds.closed() || ds.hub == nil {
		return errors.New("Can't unsubscribe")
	}
	t := ds.hub.GetTopic(ds, doctype)
	ds.removeTopic(t, id)
	return nil
}

func (ds *Subscriber) addTopic(t *topic, id string) {
	found := false
	for _, topic := range ds.topics {
		if t == topic {
			found = true
			break
		}
	}
	if !found {
		ds.topics = append(ds.topics, t)
	}
	t.subscribe <- &toWatch{&ds.Channel, id}
}

func (ds *Subscriber) removeTopic(t *topic, id string) {
	for _, topic := range ds.topics {
		if t == topic {
			t.unsubscribe <- &toWatch{&ds.Channel, id}
		}
	}
}

// closed returns true if it will no longer send events in its channel
func (ds *Subscriber) closed() bool {
	return atomic.LoadUint32(&ds.c) == 1
}

// Close closes the channel (async)
func (ds *Subscriber) Close() error {
	if !atomic.CompareAndSwapUint32(&ds.c, 0, 1) {
		return errors.New("closing a closed subscription")
	}
	// Don't block on Close
	wg := sync.WaitGroup{}
	for _, t := range ds.topics {
		wg.Add(1)
		go func(t *topic) {
			for {
				select {
				case t.unsubscribe <- &toWatch{&ds.Channel, ""}:
					wg.Done()
					return
				case <-ds.Channel:
					// Purge events
				}
			}
		}(t)
	}
	go func() {
		wg.Wait()
		close(ds.Channel)
	}()
	return nil
}
