package scheduler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo"
)

var (
	// ErrUnknownTrigger is used when the trigger type is not recognized
	ErrUnknownTrigger = errors.New("Unknown trigger type")
	// ErrNotFoundTrigger is used when the trigger was not found
	ErrNotFoundTrigger = errors.New("Trigger with specified ID does not exist")
	// ErrMalformedTrigger is used to indicate the trigger is unparsable
	ErrMalformedTrigger = echo.NewHTTPError(http.StatusBadRequest, "Trigger unparsable")
)
