package couchdb

import (
  "testing"
  "github.com/stretchr/testify/assert"
)

func TestMain(t *testing.T){
  body := []byte("{\"reason\": missing}")
  err := CouchdbError{404, body}
  assert.Contains(t, err.Error(), "404");
  assert.Contains(t, err.Error(), "missing");
}
