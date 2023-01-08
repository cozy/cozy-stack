package couchdb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCursor(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	t.Run("StartKeyCursor", func(t *testing.T) {
		req1 := &ViewRequest{
			Key: []string{"A", "B"},
		}

		c1 := NewKeyCursor(10, []string{"A", "B"}, "last-result-id")

		c1.ApplyTo(req1)
		assert.Nil(t, req1.Key)
		assert.Equal(t, []string{"A", "B"}, req1.StartKey)
		assert.Equal(t, "last-result-id", req1.StartKeyDocID)
		assert.Equal(t, 11, req1.Limit)

		c2 := NewKeyCursor(3, nil, "")

		res := &ViewResponse{
			Rows: []*ViewResponseRow{
				{Key: []string{"A", "B"}, ID: "resultA"},
				{Key: []string{"A", "B"}, ID: "resultB"},
				{Key: []string{"A", "B"}, ID: "resultC"},
				{Key: []string{"A", "B"}, ID: "resultD"},
			},
		}

		c2.UpdateFrom(res)
		assert.Len(t, res.Rows, 3)
		assert.Equal(t, []string{"A", "B"}, c2.(*StartKeyCursor).NextKey)
		assert.Equal(t, "resultD", c2.(*StartKeyCursor).NextDocID)
	})

}
