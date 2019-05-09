package oauth

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
)

// AccessCode is struct used during the OAuth2 flow. It has to be persisted in
// CouchDB, not just sent as a JSON Web Token, because it can be used only
// once (no replay attacks).
type AccessCode struct {
	Code     string `json:"_id,omitempty"`
	CouchRev string `json:"_rev,omitempty"`
	ClientID string `json:"client_id"`
	IssuedAt int64  `json:"issued_at"`
	Scope    string `json:"scope"`
}

// ID returns the access code qualified identifier
func (ac *AccessCode) ID() string { return ac.Code }

// Rev returns the access code revision
func (ac *AccessCode) Rev() string { return ac.CouchRev }

// DocType returns the access code document type
func (ac *AccessCode) DocType() string { return consts.OAuthAccessCodes }

// Clone implements couchdb.Doc
func (ac *AccessCode) Clone() couchdb.Doc { cloned := *ac; return &cloned }

// SetID changes the access code qualified identifier
func (ac *AccessCode) SetID(id string) { ac.Code = id }

// SetRev changes the access code revision
func (ac *AccessCode) SetRev(rev string) { ac.CouchRev = rev }

// CreateAccessCode an access code for the given clientID, persisted in CouchDB
func CreateAccessCode(i *instance.Instance, clientID, scope string) (*AccessCode, error) {
	ac := &AccessCode{
		ClientID: clientID,
		IssuedAt: crypto.Timestamp(),
		Scope:    scope,
	}
	if err := couchdb.CreateDoc(i, ac); err != nil {
		return nil, err
	}
	return ac, nil
}

var (
	_ couchdb.Doc = &AccessCode{}
)
