package sharing

import "github.com/cozy/cozy-stack/web/jsonapi"

// APISharing is used to serialize a Sharing to JSON-API
type APISharing struct {
	*Sharing
	// XXX Hide the credentials
	Credentials *interface{} `json:"credentials,omitempty"`
}

// Included is part of jsonapi.Object interface
func (s *APISharing) Included() []jsonapi.Object { return nil }

// Relationships is part of jsonapi.Object interface
func (s *APISharing) Relationships() jsonapi.RelationshipMap { return nil }

// Links is part of jsonapi.Object interface
func (s *APISharing) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/sharings/" + s.SID}
}

var _ jsonapi.Object = (*APISharing)(nil)
