package bitwarden

import (
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/metadata"
)

// DocTypeVersion represents the doctype version. Each time this document
// structure is modified, update this value
const DocTypeVersion = "1"

// CipherType is used to know what contains the cipher: a login, a secure note,
// a card or an identity.
type CipherType int

// LoginType, SecureNoteType, CardType, and IdentityType are the 4 possible
// types of ciphers.
const (
	LoginType      = 1
	SecureNoteType = 2
	CardType       = 3
	IdentityType   = 4
)

// LoginData is the encrypted data for a cipher with the login type.
type LoginData struct {
	URI      string `json:"uri,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	TOTP     string `json:"totp,omitempty"`
}

// MapData is used for the data of secure note, card, and identity.
type MapData map[string]interface{}

// Cipher is an encrypted item that can be a login, a secure note, a card or an
// identity.
type Cipher struct {
	CouchID  string                 `json:"_id,omitempty"`
	CouchRev string                 `json:"_rev,omitempty"`
	Type     CipherType             `json:"type"`
	Favorite bool                   `json:"favorite,omitempty"`
	Name     string                 `json:"name"`
	Notes    string                 `json:"notes,omitempty"`
	FolderID string                 `json:"folder_id,omitempty"`
	Login    *LoginData             `json:"login,omitempty"`
	Data     *MapData               `json:"data,omitempty"`
	Metadata *metadata.CozyMetadata `json:"cozyMetadata,omitempty"`
}

// ID returns the cipher qualified identifier
func (c *Cipher) ID() string { return c.CouchID }

// Rev returns the cipher revision
func (c *Cipher) Rev() string { return c.CouchRev }

// DocType returns the cipher document type
func (c *Cipher) DocType() string { return consts.BitwardenCiphers }

// Clone implements couchdb.Doc
func (c *Cipher) Clone() couchdb.Doc {
	cloned := *c
	if c.Login != nil {
		cloned.Login = &LoginData{
			URI:      c.Login.URI,
			Username: c.Login.Username,
			Password: c.Login.Password,
			TOTP:     c.Login.TOTP,
		}
	}
	return &cloned
}

// SetID changes the cipher qualified identifier
func (c *Cipher) SetID(id string) { c.CouchID = id }

// SetRev changes the cipher revision
func (c *Cipher) SetRev(rev string) { c.CouchRev = rev }

var _ couchdb.Doc = &Cipher{}
