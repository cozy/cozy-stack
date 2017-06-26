package realtime

import (
	"encoding/json"
	"sync"

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
	json.Marshaler
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

	// Subscribe adds a listener for events on a given type
	// it returns an EventChannel, call the EventChannel Close method
	// to Unsubscribe.
	Subscribe(domain, topicName string) EventChannel

	// SubscribeLocalAll adds a listener for all events that happened in this
	// cozy-stack process.
	SubscribeLocalAll() EventChannel
}

// EventChannel is returned when Subscribing to the hub
type EventChannel interface {
	// Read returns a chan for events
	Read() <-chan *Event
	// Close closes the channel
	Close() error
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
