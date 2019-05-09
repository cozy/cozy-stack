package job

import (
	"errors"
	"net/http"

	"github.com/cozy/echo"
)

var (
	// ErrClosed is using a closed system
	ErrClosed = errors.New("jobs: closed")
	// ErrNotFoundJob is used when the job could not be found
	ErrNotFoundJob = errors.New("jobs: not found")
	// ErrQueueClosed is used to indicate the queue is closed
	ErrQueueClosed = errors.New("jobs: queue is closed")
	// ErrUnknownWorker the asked worker does not exist
	ErrUnknownWorker = errors.New("jobs: could not find worker")
	// ErrMessageNil is used for an nil message
	ErrMessageNil = errors.New("jobs: message is nil")
	// ErrMessageUnmarshal is used when unmarshalling a message causes an error
	ErrMessageUnmarshal = errors.New("jobs: message unmarshal")
	// ErrAbort can be used to abort the execution of the job without causing
	// errors.
	ErrAbort = errors.New("jobs: abort")

	// ErrUnknownTrigger is used when the trigger type is not recognized
	ErrUnknownTrigger = errors.New("Unknown trigger type")
	// ErrNotFoundTrigger is used when the trigger was not found
	ErrNotFoundTrigger = errors.New("Trigger with specified ID does not exist")
	// ErrMalformedTrigger is used to indicate the trigger is unparsable
	ErrMalformedTrigger = echo.NewHTTPError(http.StatusBadRequest, "Trigger unparsable")
)

// ErrBadTrigger is an error conveying the information of a trigger that is not
// valid, and could be deleted.
type ErrBadTrigger struct {
	Err error
}

func (e ErrBadTrigger) Error() string {
	return e.Err.Error()
}
