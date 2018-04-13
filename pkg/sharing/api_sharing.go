package sharing

import (
	"encoding/json"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/web/jsonapi"
)

// APISharing is used to serialize a Sharing to JSON-API
type APISharing struct {
	*Sharing
	// XXX Hide the credentials
	Credentials *interface{}           `json:"credentials,omitempty"`
	SharedDocs  []couchdb.DocReference `json:"shared_docs,omitempty"`
}

// Included is part of jsonapi.Object interface
func (s *APISharing) Included() []jsonapi.Object { return nil }

// Relationships is part of jsonapi.Object interface
func (s *APISharing) Relationships() jsonapi.RelationshipMap {
	return jsonapi.RelationshipMap{
		"shared_docs": jsonapi.Relationship{
			Data: s.SharedDocs,
		},
	}
}

// MarshalJSON is part of jsonapi.Object interface
func (s *APISharing) MarshalJSON() ([]byte, error) {
	ref := s.SharedDocs
	s.SharedDocs = nil
	res, err := json.Marshal(s.Sharing)
	s.SharedDocs = ref
	return res, err
}

// Links is part of jsonapi.Object interface
func (s *APISharing) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/sharings/" + s.SID}
}

var _ jsonapi.Object = (*APISharing)(nil)

// APICredentials is used to serialize credentials to JSON-API. It is used for
// Cozy to Cozy exchange of the credentials, after a recipient has accepted a
// sharing.
type APICredentials struct {
	*Credentials
	CID string `json:"_id,omitempty"`
}

// ID returns the sharing qualified identifier
func (c *APICredentials) ID() string { return c.CID }

// Rev returns the sharing revision
func (c *APICredentials) Rev() string { return "" }

// DocType returns the sharing document type
func (c *APICredentials) DocType() string { return consts.SharingsAnswer }

// SetID changes the sharing qualified identifier
func (c *APICredentials) SetID(id string) { c.CID = id }

// SetRev changes the sharing revision
func (c *APICredentials) SetRev(rev string) {}

// Clone is part of jsonapi.Object interface
func (c *APICredentials) Clone() couchdb.Doc {
	panic("APICredentials must not be cloned")
}

// Included is part of jsonapi.Object interface
func (c *APICredentials) Included() []jsonapi.Object { return nil }

// Relationships is part of jsonapi.Object interface
func (c *APICredentials) Relationships() jsonapi.RelationshipMap { return nil }

// Links is part of jsonapi.Object interface
func (c *APICredentials) Links() *jsonapi.LinksList { return nil }

var _ jsonapi.Object = (*APICredentials)(nil)
