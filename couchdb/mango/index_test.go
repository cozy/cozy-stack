package mango

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIndexMarshaling(t *testing.T) {
	def := IndexOnFields("folderID", "name")
	jsonbytes, _ := json.Marshal(def)
	expected := `{"index":{"fields":["folderID","name"]}}`
	assert.Equal(t, expected, string(jsonbytes), "index should MarshalJSON properly")
}
