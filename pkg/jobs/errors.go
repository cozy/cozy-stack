package jobs

import "errors"

var (
	// ErrQueueClosed is used to indicate the queue is closed
	ErrQueueClosed = errors.New("Queue is closed")
	// ErrUnknownWorker the asked worker does not exist
	ErrUnknownWorker = errors.New("Could not find worker")
	// ErrUnknownMessageType is used for an unknown message encoding type
	ErrUnknownMessageType = errors.New("Unknown message encoding type")
)
