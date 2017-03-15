package sharings

import (
	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/web/jsonapi"
)

// Recipient is a struct describing a sharing recipient
type Recipient struct {
	RID    string `json:"_id,omitempty"`
	RRev   string `json:"_rev,omitempty"`
	Email  string `json:"email"`
	URL    string `json:"url"`
	Client *auth.Client
}

// ID returns the recipient qualified identifier
func (r *Recipient) ID() string { return r.RID }

// Rev returns the recipient revision
func (r *Recipient) Rev() string { return r.RRev }

// DocType returns the recipient document type
func (r *Recipient) DocType() string { return consts.Recipients }

// SetID changes the recipient qualified identifier
func (r *Recipient) SetID(id string) { r.RID = id }

// SetRev changes the recipient revision
func (r *Recipient) SetRev(rev string) { r.RRev = rev }

// Relationships implements jsonapi.Doc
func (r *Recipient) Relationships() jsonapi.RelationshipMap { return nil }

// Included implements jsonapi.Doc
func (r *Recipient) Included() []jsonapi.Object { return nil }

// Links implements jsonapi.Doc
func (r *Recipient) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/recipients/" + r.RID}
}

var (
	_ couchdb.Doc    = &Recipient{}
	_ jsonapi.Object = &Recipient{}
)
