package mango

import "encoding/json"

// An IndexDefinition is just a list of fields to be indexed.
type IndexDefinition []string

// MarshalJSON implements the json.Marshaller interface on IndexDefinition
// by wrapping it in a {"fields": ...}
func (def IndexDefinition) MarshalJSON() ([]byte, error) {
	return json.Marshal(makeMap("fields", []string(def)))
}

// An IndexDefinitionRequest is a request to be POSTED to create the index
type IndexDefinitionRequest struct {
	Name  string          `json:"name,omitempty"`
	DDoc  string          `json:"ddoc,omitempty"`
	Index IndexDefinition `json:"index"`
}

// IndexOnFields constructs a new IndexDefinitionRequest
// it lets couchdb defaults for index & designdoc names.
func IndexOnFields(fields ...string) IndexDefinitionRequest {
	return IndexDefinitionRequest{
		Index: IndexDefinition(fields),
	}
}
