package realtime

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// Basic data events
const (
	EventCreate = "CREATED"
	EventUpdate = "UPDATED"
	EventDelete = "DELETED"
)

// Doc is an interface for a object with DocType, ID
type Doc interface {
	ID() string
	DocType() string
}

// Event is the basic message structure manipulated by the realtime package
type Event struct {
	Domain string `json:"domain"`
	Prefix string `json:"prefix,omitempty"`
	Verb   string `json:"verb"`
	Doc    Doc    `json:"doc"`
	OldDoc Doc    `json:"old,omitempty"`
}

func newEvent(db prefixer.Prefixer, verb string, doc Doc, oldDoc Doc) *Event {
	return &Event{
		Domain: db.DomainName(),
		Prefix: db.DBPrefix(),
		Verb:   verb,
		Doc:    doc,
		OldDoc: oldDoc,
	}
}

// DBPrefix implements the prefixer.Prefixer interface.
func (e *Event) DBPrefix() string {
	if e.Prefix != "" {
		return e.Prefix
	}
	return e.Domain
}

// DomainName implements the prefixer.Prefixer interface.
func (e *Event) DomainName() string {
	return e.Domain
}

// The following API is inspired by https://github.com/gocontrib/pubsub

// Hub is an object which recive events and calls appropriate listener
type Hub interface {
	// Emit is used by publishers when an event occurs
	Publish(db prefixer.Prefixer, verb string, doc Doc, oldDoc Doc)

	// Subscriber creates a DynamicSubscriber that can subscribe to several
	// doctypes. Call its Close method to Unsubscribe.
	Subscriber(prefixer.Prefixer) *DynamicSubscriber

	// SubscribeLocalAll adds a listener for all events that happened in this
	// cozy-stack process.
	SubscribeLocalAll() *DynamicSubscriber

	// GetTopic returns the topic for the given domain+doctype.
	// It creates the topic if it does not exist.
	GetTopic(db prefixer.Prefixer, doctype string) *topic
}

// MemSub is a chan of events
type MemSub chan *Event

// DynamicSubscriber is used to subscribe to several doctypes
type DynamicSubscriber struct {
	prefixer.Prefixer
	Channel MemSub
	hub     Hub
	topics  []*topic
	c       uint32 // mark whether or not the sub is closed
}

func newDynamicSubscriber(hub Hub, db prefixer.Prefixer) *DynamicSubscriber {
	return &DynamicSubscriber{
		Prefixer: db,
		Channel:  make(chan *Event, 10),
		hub:      hub,
	}
}

// Subscribe adds a listener for events on a whole doctype
func (ds *DynamicSubscriber) Subscribe(doctype string) error {
	if ds.Closed() || ds.hub == nil {
		return errors.New("Can't subscribe")
	}
	t := ds.hub.GetTopic(ds, doctype)
	ds.addTopic(t, "")
	return nil
}

// Watch adds a listener for events for a specific document (doctype+id)
func (ds *DynamicSubscriber) Watch(doctype, id string) error {
	if ds.Closed() || ds.hub == nil {
		return errors.New("Can't subscribe")
	}
	t := ds.hub.GetTopic(ds, doctype)
	ds.addTopic(t, id)
	return nil
}

func (ds *DynamicSubscriber) addTopic(t *topic, id string) {
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

// Closed returns true if it will no longer send events in its channel
func (ds *DynamicSubscriber) Closed() bool {
	return atomic.LoadUint32(&ds.c) == 1
}

// Close closes the channel (async)
func (ds *DynamicSubscriber) Close() error {
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
				case t.unsubscribe <- &ds.Channel:
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

var globalHubMu sync.Mutex
var globalHub Hub

// GetHub returns the global hub
func GetHub() Hub {
	globalHubMu.Lock()
	defer globalHubMu.Unlock()
	if globalHub != nil {
		return globalHub
	}
	cli := config.GetConfig().Realtime.Client()
	if cli == nil {
		globalHub = newMemHub()
	} else {
		globalHub = newRedisHub(cli)
	}
	return globalHub
}
