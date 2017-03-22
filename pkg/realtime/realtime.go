package realtime

// Basic data events
const (
	EventCreate = "CREATED"
	EventUpdate = "UPDATED"
	EventDelete = "DELETED"
)

// Doc is an interface for a object with DocType, ID, Rev
type Doc interface {
	ID() string
	Rev() string
	DocType() string
}

// Event is the basic message structure manipulated by the realtime package
type Event struct {
	Instance string
	Type     string
	Doc      Doc
}

// The following API is inspired by https://github.com/gocontrib/pubsub

// Hub is an object which recive events and calls appropriate listener
type Hub interface {
	// Emit is used by publishers when an event occurs
	Publish(*Event)

	// Subscribe adds a listener for events on a given type
	// it returns an EventChannel, call the EventChannel Close method
	// to Unsubscribe.
	Subscribe(string) EventChannel
}

// EventChannel is returned when Suscribing to the hub
type EventChannel interface {
	// Read returns a chan for events
	Read() <-chan *Event
	// Close closes the channel
	Close() error
}
