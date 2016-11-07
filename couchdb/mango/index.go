package mango

import "encoding/json"

// An IndexFields is just a list of fields to be indexed.
type IndexFields []string

// MarshalJSON implements the json.Marshaller interface on IndexFields
// by wrapping it in a {"fields": ...}
func (def IndexFields) MarshalJSON() ([]byte, error) {
	return json.Marshal(makeMap("fields", []string(def)))
}

// NewIndexFields returns an IndexFields from a list of string
// arguments
func NewIndexFields(fields ...string) IndexFields {
	return IndexFields(fields)
}

// An Index is a request to be POSTED to create the index
type Index struct {
	Name  string      `json:"name,omitempty"`
	DDoc  string      `json:"ddoc,omitempty"`
	Index IndexFields `json:"index"`
}

// IndexOnFields constructs a new Index
// it lets couchdb defaults for index & designdoc names.
func IndexOnFields(fields ...string) Index {
	return Index{
		Index: IndexFields(fields),
	}
}
