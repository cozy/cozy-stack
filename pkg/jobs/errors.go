package jobs

import (
	"errors"
	"net/http"

	"github.com/labstack/echo"
)

var (
	// ErrQueueClosed is used to indicate the queue is closed
	ErrQueueClosed = errors.New("Queue is closed")
	// ErrUnknownWorker the asked worker does not exist
	ErrUnknownWorker = errors.New("Could not find worker")
	// ErrUnknownMessageType is used for an unknown message encoding type
	ErrUnknownMessageType = errors.New("Unknown message encoding type")
	// ErrUnknownTrigger is used when the trigger type is not recognized
	ErrUnknownTrigger = errors.New("Unknown trigger type")
	// ErrNotFoundTrigger is used when the trigger was not found
	ErrNotFoundTrigger = errors.New("Trigger with specified ID does not exist")
	// ErrMalformedTrigger is used to indicate the trigger is unparsable
	ErrMalformedTrigger = echo.NewHTTPError(http.StatusBadRequest, "Trigger unparsable")
)
