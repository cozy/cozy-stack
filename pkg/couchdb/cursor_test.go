package couchdb

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCursor(t *testing.T) {

	req1 := &ViewRequest{
		Key: []string{"A", "B"},
	}

	c1 := &Cursor{
		Limit:     10,
		NextKey:   []string{"A", "B"},
		NextDocID: "last-result-id",
	}

	req2 := c1.ApplyTo(req1)
	assert.Nil(t, req2.Key)
	assert.Equal(t, []string{"A", "B"}, req2.StartKey)
	assert.Equal(t, "last-result-id", req2.StartKeyDocID)
	assert.Equal(t, 11, req2.Limit)

	c2 := &Cursor{
		Limit: 3,
	}

	res := &ViewResponse{
		Rows: []struct {
			ID    string           `json:"id"`
			Key   interface{}      `json:"key"`
			Value interface{}      `json:"value"`
			Doc   *json.RawMessage `json:"doc"`
		}{
			{Key: []string{"A", "B"}, ID: "resultA"},
			{Key: []string{"A", "B"}, ID: "resultB"},
			{Key: []string{"A", "B"}, ID: "resultC"},
			{Key: []string{"A", "B"}, ID: "resultD"},
		},
	}

	c2.UpdateFrom(res)
	assert.Len(t, res.Rows, 3)
	assert.Equal(t, []string{"A", "B"}, c2.NextKey)
	assert.Equal(t, "resultD", c2.NextDocID)

}
