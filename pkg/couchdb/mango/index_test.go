package mango

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIndexMarshaling(t *testing.T) {
	def := IndexOnFields("io.cozy.foo", "dir_id", "name")
	jsonbytes, _ := json.Marshal(def.Request)
	expected := `{"index":{"fields":["dir_id","name"]}}`
	assert.Equal(t, expected, string(jsonbytes), "index should MarshalJSON properly")
}
