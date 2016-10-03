package couchdb

import (
	"fmt"
)

// Error represent an error from couchdb
type Error struct {
	StatusCode  int
	CouchdbJSON []byte
}

func (e *Error) Error() string {
	return fmt.Sprintf("CouchdbError %d : %s", e.StatusCode, e.CouchdbJSON)
}
