package couchdb

import (
	"bytes"
	"fmt"
	"net/http"
)

// Error represent an error from couchdb
type Error struct {
	StatusCode  int
	CouchdbJSON []byte
}

func (e *Error) Error() string {
	return fmt.Sprintf("CouchdbError %d : %s", e.StatusCode, e.CouchdbJSON)
}

func isNoDatabaseError(err error) bool {
	if err == nil {
		return false
	}
	if coucherr, ok := err.(*Error); ok {
		return bytes.Contains(coucherr.CouchdbJSON, []byte("no_db_file"))
	}
	return false
}

func newConnectionError(orignalError error) error {
	return &Error{http.StatusServiceUnavailable,
		[]byte("{\"error\":\"No couch to seat on.\"}")}
}

func newIOReadError(originalError error) error {
	return &Error{http.StatusServiceUnavailable,
		[]byte("{\"error\":\"Couchdb hangup.\"}")}
}

func newRequestError(originalError error) error {
	return &Error{http.StatusInternalServerError,
		[]byte("{\"error\":\"Wrong configuration for couchdbserver\"}")}
}

func newCouchdbError(statusCode int, couchdbJSON []byte) error {
	return &Error{statusCode, couchdbJSON}
}
