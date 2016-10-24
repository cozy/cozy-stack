package jsonapi

import (
	"encoding/json"

	"github.com/cozy/cozy-stack/couchdb"
)

// Object is an interface to serialize something to a JSON-API Object
type Object interface {
	couchdb.Doc
	SelfLink() string
	Relationships() RelationshipMap
	Included() []Object
}

// Meta is a container for the couchdb revision, in JSON-API land
type Meta struct {
	Rev string `json:"rev"`
}

// LinksList is the common links used in JSON-API for the top-level or a
// resource object
// See http://jsonapi.org/format/#document-links
type LinksList struct {
	Self    string `json:"self,omitempty"`
	Related string `json:"related,omitempty"`
	Prev    string `json:"prev,omitempty"`
	Next    string `json:"next,omitempty"`
}

// ResourceIdentifier is an object, used in relationships, to identify an
// individual resource
// See http://jsonapi.org/format/#document-resource-identifier-objects
type ResourceIdentifier struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// Relationship is a resource linkage, as described in JSON-API
// See http://jsonapi.org/format/#document-resource-object-relationships
//
// Data can be a single ResourceIdentifier for to-one relationships,
// or an array of them for to-many relationships.
type Relationship struct {
	Links *LinksList  `json:"links,omitempty"`
	Data  interface{} `json:"data"`
}

// RelationshipMap is a map of relationships
// See http://jsonapi.org/format/#document-resource-object-relationships
type RelationshipMap map[string]Relationship

// ObjectMarshalling is a JSON-API object
// See http://jsonapi.org/format/#document-resource-objects
type ObjectMarshalling struct {
	Type          string          `json:"type"`
	ID            string          `json:"id"`
	Attributes    interface{}     `json:"attributes"`
	Meta          Meta            `json:"meta"`
	Links         LinksList       `json:"links,omitempty"`
	Relationships RelationshipMap `json:"relationships,omitempty"`
}

// MarshalObject serializes an Object to JSON.
// It returns a json.RawMessage that can be used a in Document.
func MarshalObject(o Object) (json.RawMessage, error) {
	id := o.ID()
	rev := o.Rev()
	self := o.SelfLink()
	rels := o.Relationships()

	o.SetID("")
	o.SetRev("")
	defer func() {
		o.SetID(id)
		o.SetRev(rev)
	}()

	data := ObjectMarshalling{
		Type:          o.DocType(),
		ID:            id,
		Attributes:    o,
		Meta:          Meta{Rev: rev},
		Links:         LinksList{Self: self},
		Relationships: rels,
	}
	return json.Marshal(data)
}
