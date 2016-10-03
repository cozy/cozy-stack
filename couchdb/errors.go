package couchdb

import (
  "fmt"
)

type CouchdbError struct {
  StatusCode int
  CouchdbJSON []byte
}

func (e *CouchdbError) Error() string {
    return fmt.Sprintf("CouchdbError %d : %s", e.StatusCode, e.CouchdbJSON)
}
