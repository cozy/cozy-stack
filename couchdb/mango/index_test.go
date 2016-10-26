package mango

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIndexMarshaling(t *testing.T) {
	def := IndexOnFields("folder_id", "name")
	jsonbytes, _ := json.Marshal(def)
	expected := `{"index":{"fields":["folder_id","name"]}}`
	assert.Equal(t, expected, string(jsonbytes), "index should MarshalJSON properly")
}
