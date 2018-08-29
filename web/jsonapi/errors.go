package jsonapi

import (
	"fmt"
	"net/http"
	"strconv"
)

// SourceError contains references to the source of the error
type SourceError struct {
	Pointer   string `json:"pointer,omitempty"`
	Parameter string `json:"parameter,omitempty"`
}

// Error objects provide additional information about problems encountered
// while performing an operation.
// See http://jsonapi.org/format/#error-objects
type Error struct {
	Status int         `json:"status,string"`
	Title  string      `json:"title"`
	Code   string      `json:"code,omitempty"`
	Detail string      `json:"detail,omitempty"`
	Source SourceError `json:"source,omitempty"`
	Links  *LinksList  `json:"links,omitempty"`
}

// ErrorList is just an array of error objects
type ErrorList []*Error

func (e *Error) Error() string {
	return e.Title + "(" + strconv.Itoa(e.Status) + ")" + ": " + e.Detail
}

// NewError creates a new generic Error
func NewError(status int, detail string) *Error {
	return &Error{
		Status: status,
		Title:  http.StatusText(status),
		Detail: detail,
	}
}

// Errorf creates a new generic Error with detail build as Sprintf
func Errorf(status int, format string, args ...interface{}) *Error {
	detail := fmt.Sprintf(format, args...)
	return NewError(status, detail)
}

// NotFound returns a 404 formatted error
func NotFound(err error) *Error {
	return &Error{
		Status: http.StatusNotFound,
		Title:  "Not Found",
		Detail: err.Error(),
	}
}

// BadRequest returns a 400 formatted error
func BadRequest(err error) *Error {
	return &Error{
		Status: http.StatusBadRequest,
		Title:  "Bad request",
		Detail: err.Error(),
	}
}

// BadJSON returns a 400 formatted error meaning the json input is
// malformed.
func BadJSON() *Error {
	return &Error{
		Status: http.StatusBadRequest,
		Title:  "Bad request",
		Detail: "JSON input is malformed or is missing mandatory fields",
	}
}

// MethodNotAllowed returns a 405 formatted error
func MethodNotAllowed(method string) *Error {
	return &Error{
		Status: http.StatusMethodNotAllowed,
		Title:  "Method Not Allowed",
		Detail: method + " is not allowed on this endpoint",
	}
}

// Conflict returns a 409 formatted error representing a conflict
func Conflict(err error) *Error {
	return &Error{
		Status: http.StatusConflict,
		Title:  "Conflict",
		Detail: err.Error(),
	}
}

// InternalServerError returns a 500 formatted error
func InternalServerError(err error) *Error {
	return &Error{
		Status: http.StatusInternalServerError,
		Title:  "Internal Server Error",
		Detail: err.Error(),
	}
}

// PreconditionFailed returns a 412 formatted error when an expectation from an
// HTTP header is not matched
func PreconditionFailed(parameter string, err error) *Error {
	return &Error{
		Status: http.StatusPreconditionFailed,
		Title:  "Precondition Failed",
		Detail: err.Error(),
		Source: SourceError{
			Parameter: parameter,
		},
	}
}

// InvalidParameter returns a 422 formatted error when an HTTP or Query-String
// parameter is invalid
func InvalidParameter(parameter string, err error) *Error {
	return &Error{
		Status: http.StatusUnprocessableEntity,
		Title:  "Invalid Parameter",
		Detail: err.Error(),
		Source: SourceError{
			Parameter: parameter,
		},
	}
}

// InvalidAttribute returns a 422 formatted error when an attribute is invalid
func InvalidAttribute(attribute string, err error) *Error {
	return &Error{
		Status: http.StatusUnprocessableEntity,
		Title:  "Invalid Attribute",
		Detail: err.Error(),
		Source: SourceError{
			Pointer: "/data/attributes/" + attribute,
		},
	}
}

// Forbidden returns a 403 Forbidden error formatted when an action is
// fobidden.
func Forbidden(err error) *Error {
	return &Error{
		Status: http.StatusForbidden,
		Title:  "Forbidden",
		Detail: err.Error(),
	}
}

// BadGateway returns a 502 formatted error
func BadGateway(err error) *Error {
	return &Error{
		Status: http.StatusBadGateway,
		Title:  "Bad Gateway",
		Detail: err.Error(),
	}
}
