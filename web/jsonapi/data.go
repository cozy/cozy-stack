package jsonapi

import (
	"encoding/json"

	"github.com/cozy/cozy-stack/couchdb"
)

// Object is an interface to serialize something to a JSON-API Object
type Object interface {
	couchdb.Doc
	// TODO links, relationships
}

// Meta is a container for the couchdb revision, in JSON-API land
type Meta struct {
	Rev string `json:"rev"`
}

// ObjectMarshalling is a JSON-API object
// See http://jsonapi.org/format/#document-resource-objects
type ObjectMarshalling struct {
	Type       string      `json:"type"`
	ID         string      `json:"id"`
	Attributes interface{} `json:"attributes"`
	Meta       Meta        `json:"meta"`
}

// MarshalObject serializes an Object to JSON.
// It returns a json.RawMessage that can be used a in Document.
func MarshalObject(o Object) (json.RawMessage, error) {
	id := o.ID()
	o.SetID("")
	rev := o.Rev()
	o.SetRev("")
	defer func() {
		o.SetID(id)
		o.SetRev(rev)
	}()
	data := ObjectMarshalling{
		Type:       o.DocType(),
		ID:         id,
		Attributes: o,
		Meta:       Meta{Rev: rev},
	}
	return json.Marshal(data)
}
