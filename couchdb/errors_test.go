package couchdb

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestError_JSON(t *testing.T) {
	couchError := Error{
		StatusCode: 200,
		CouchdbJSON: []byte(`{
			"hello": "couchdb"
		}`),
		Name:     "a name",
		Reason:   "a reason",
		Original: fmt.Errorf("universe %d", 42),
	}

	asJSON := couchError.JSON()

	expectedMap := map[string]interface{}{
		"ok":       false,
		"status":   "200",
		"error":    "a name",
		"reason":   "a reason",
		"original": "universe 42",
	}

	assert.EqualValues(t, expectedMap, asJSON)
}
