package bitwarden

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/metadata"
)

// Contact is used to add users to an organization.
type Contact struct {
	UserID    string                `json:"_id,omitempty"`
	CouchRev  string                `json:"_rev,omitempty"`
	Email     string                `json:"email"`
	PublicKey string                `json:"public_key"`
	Confirmed bool                  `json:"confirmed,omitempty"`
	Metadata  metadata.CozyMetadata `json:"cozyMetadata"`
}

// ID returns the contact identifier
func (c *Contact) ID() string { return c.UserID }

// Rev returns the contact revision
func (c *Contact) Rev() string { return c.CouchRev }

// SetID changes the contact identifier
func (c *Contact) SetID(id string) { c.UserID = id }

// SetRev changes the contact revision
func (c *Contact) SetRev(rev string) { c.CouchRev = rev }

// DocType returns the contact document type
func (c *Contact) DocType() string { return consts.BitwardenContacts }

// Clone implements couchdb.Doc
func (c *Contact) Clone() couchdb.Doc {
	cloned := *c
	return &cloned
}

var _ couchdb.Doc = &Contact{}
