package mango

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIndexMarshaling(t *testing.T) {
	t.Run("WithFields", func(t *testing.T) {
		def := MakeIndex("io.cozy.foo", "my-index", IndexDef{Fields: []string{"dir_id", "name"}})
		jsonbytes, _ := json.Marshal(def.Request)
		expected := `{"ddoc":"my-index","index":{"fields":["dir_id","name"]}}`
		assert.Equal(t, expected, string(jsonbytes), "index should MarshalJSON properly")
	})

	t.Run("WithFieldsAndPartialFilter", func(t *testing.T) {
		def := MakeIndex("io.cozy.foo", "my-index", IndexDef{Fields: []string{"dir_id", "name"}, PartialFilter: Exists("trashed")})
		jsonbytes, _ := json.Marshal(def.Request)
		expected := `{"ddoc":"my-index","index":{"fields":["dir_id","name"],"partial_filter_selector":{"trashed":{"$exists":true}}}}`
		assert.Equal(t, expected, string(jsonbytes), "index should MarshalJSON properly")
	})
}
