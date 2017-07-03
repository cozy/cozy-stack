package realtime

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/cozy/cozy-stack/pkg/config"
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
	Verb   string `json:"verb"`
	Doc    Doc    `json:"doc"`
	OldDoc Doc    `json:"old,omitempty"`
}

// The following API is inspired by https://github.com/gocontrib/pubsub

// Hub is an object which recive events and calls appropriate listener
type Hub interface {
	// Emit is used by publishers when an event occurs
	Publish(event *Event)

	// Subscriber creates a DynamicSubscriber that can subscribe to several
	// doctypes. Call its Close method to Unsubscribe.
	Subscriber(domain string) *DynamicSubscriber

	// SubscribeLocalAll adds a listener for all events that happened in this
	// cozy-stack process.
	SubscribeLocalAll() *DynamicSubscriber

	// GetTopic returns the topic for the given domain+doctype.
	// It creates the topic if it does not exist.
	GetTopic(domain, doctype string) *topic
}

// MemSub is a chan of events
type MemSub chan *Event

// DynamicSubscriber is used to subscribe to several doctypes
type DynamicSubscriber struct {
	Channel MemSub
	Domain  string
	hub     Hub
	topics  []*topic
	c       uint32 // mark whether or not the sub is closed
}

func newDynamicSubscriber(hub Hub, domain string) *DynamicSubscriber {
	return &DynamicSubscriber{
		Channel: make(chan *Event, 10),
		hub:     hub,
		Domain:  domain,
	}
}

// Subscribe adds a listener for events on a given doctype
func (ds *DynamicSubscriber) Subscribe(doctype string) error {
	if ds.Closed() || ds.hub == nil {
		return errors.New("Can't subscribe")
	}
	t := ds.hub.GetTopic(ds.Domain, doctype)
	// TODO check that t is not already in ds.topics
	ds.addTopic(t)
	return nil
}

func (ds *DynamicSubscriber) addTopic(t *topic) {
	ds.topics = append(ds.topics, t)
	t.subscribe <- &ds.Channel
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
			t.unsubscribe <- &ds.Channel
			wg.Done()
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
