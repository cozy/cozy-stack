package realtime

import (
	"sync"

	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/prefixer"
)

// Basic data events
const (
	EventCreate = "CREATED"
	EventUpdate = "UPDATED"
	EventDelete = "DELETED"
	EventNotify = "NOTIFIED"
)

// Doc is an interface for a object with DocType, ID
type Doc interface {
	ID() string
	DocType() string
}

// Event is the basic message structure manipulated by the realtime package
type Event struct {
	Cluster int    `json:"cluster,omitempty"`
	Domain  string `json:"domain"`
	Prefix  string `json:"prefix,omitempty"`
	Verb    string `json:"verb"`
	Doc     Doc    `json:"doc"`
	OldDoc  Doc    `json:"old,omitempty"`
}

func newEvent(db prefixer.Prefixer, verb string, doc Doc, oldDoc Doc) *Event {
	return &Event{
		Cluster: db.DBCluster(),
		Domain:  db.DomainName(),
		Prefix:  db.DBPrefix(),
		Verb:    verb,
		Doc:     doc,
		OldDoc:  oldDoc,
	}
}

// DBCluster implements the prefixer.Prefixer interface.
func (e *Event) DBCluster() int {
	return e.Cluster
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

	// Subscriber creates a Subscriber that can subscribe to several
	// doctypes. Call its Close method to Unsubscribe.
	Subscriber(prefixer.Prefixer) *Subscriber

	// SubscribeFirehose adds a listener for all events that happened in this
	// cozy-stack process.
	SubscribeFirehose() *Subscriber

	subscribe(sub *Subscriber, key string)
	unsubscribe(sub *Subscriber, key string)
	watch(sub *Subscriber, key, id string)
	unwatch(sub *Subscriber, key, id string)
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
