package mango

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

type M map[string]interface{}
type S []interface{}

func DeepEqual(t *testing.T, map1, map2 interface{}) bool {
	j1, err1 := json.Marshal(map1)
	j2, err2 := json.Marshal(map2)
	if assert.NoError(t, err1) && assert.NoError(t, err2) {
		assert.Equal(t, string(j1), string(j2))
	}
	return false
}

func TestQueryMarshaling(t *testing.T) {
	q1 := Equal("DirID", "ab123")
	DeepEqual(t, q1.ToMango(), M{"DirID": "ab123"})
	q2 := Gt("Size", 1000)
	DeepEqual(t, q2.ToMango(), M{"Size": M{"$gt": 1000}})
	q3 := And(q1, q2)
	DeepEqual(t, q3.ToMango(),
		M{"$and": S{
			M{"DirID": "ab123"},
			M{"Size": M{"$gt": 1000}},
		}})

	q4 := Not(Equal("DirID", "ab123"))
	DeepEqual(t, q4.ToMango(), M{"$not": M{"DirID": "ab123"}})
}

func TestSortMarshaling(t *testing.T) {
	s1 := SortBy{
		{"dir_id", Asc},
		{"foo_bar", Desc},
	}
	j1, err := json.Marshal(s1)
	if assert.NoError(t, err) {
		assert.Equal(t, j1, []byte(`[{"dir_id":"asc"},{"foo_bar":"desc"}]`))
	}
}
