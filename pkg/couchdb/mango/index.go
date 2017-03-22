package mango

import "encoding/json"

// An IndexFields is just a list of fields to be indexed.
type IndexFields []string

// MarshalJSON implements the json.Marshaller interface on IndexFields
// by wrapping it in a {"fields": ...}
func (def IndexFields) MarshalJSON() ([]byte, error) {
	return json.Marshal(makeMap("fields", []string(def)))
}

// IndexRequest is a request to be POSTED to create the index
type IndexRequest struct {
	Name  string      `json:"name,omitempty"`
	DDoc  string      `json:"ddoc,omitempty"`
	Index IndexFields `json:"index"`
}

// Index contains an index request on a specified domain.
type Index struct {
	Doctype string
	Request *IndexRequest
}

// IndexOnFields constructs a new Index
// it lets couchdb defaults for index & designdoc names.
func IndexOnFields(doctype, name string, fields []string) *Index {
	return &Index{
		Doctype: doctype,
		Request: &IndexRequest{
			DDoc:  name,
			Index: IndexFields(fields),
		},
	}
}
