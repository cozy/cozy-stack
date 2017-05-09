package jobs

import "errors"

var (
	// ErrNotFoundJob is used when the job could not be found
	ErrNotFoundJob = errors.New("jobs: not found")
	// ErrQueueClosed is used to indicate the queue is closed
	ErrQueueClosed = errors.New("jobs: queue is closed")
	// ErrUnknownWorker the asked worker does not exist
	ErrUnknownWorker = errors.New("jobs: could not find worker")
	// ErrUnknownMessageType is used for an unknown message encoding type
	ErrUnknownMessageType = errors.New("jobs: unknown message encoding type")
)
