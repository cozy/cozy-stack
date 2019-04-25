package sharing

import (
	"net/http"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/echo"
)

// InfoByDocTypeData returns the sharings info as data array in the JSON-API format
func InfoByDocTypeData(c echo.Context, statusCode int, sharings []*APISharing) error {
	data := make([]jsonapi.Object, len(sharings))
	for i, s := range sharings {
		data[i] = s
	}
	return jsonapi.DataList(c, http.StatusOK, data, nil)
}

// APISharing is used to serialize a Sharing to JSON-API
type APISharing struct {
	*Sharing
	// XXX Hide the credentials
	Credentials *interface{}           `json:"credentials,omitempty"`
	SharedDocs  []couchdb.DocReference `json:"-"`
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

// Links is part of jsonapi.Object interface
func (s *APISharing) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/sharings/" + s.SID}
}

// Clone is part of the couchdb.Doc interface
func (s *APISharing) Clone() couchdb.Doc {
	panic("APISharing should not be cloned")
}

var _ jsonapi.Object = (*APISharing)(nil)

// APICredentials is used to serialize credentials to JSON-API. It is used for
// Cozy to Cozy exchange of the credentials, after a recipient has accepted a
// sharing.
type APICredentials struct {
	*Credentials
	PublicName string `json:"public_name,omitempty"`
	CID        string `json:"_id,omitempty"`
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
