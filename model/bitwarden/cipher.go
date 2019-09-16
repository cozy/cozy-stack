package bitwarden

import (
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
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

// LoginURI is a field for an URI.
// See https://github.com/bitwarden/jslib/blob/master/src/models/api/loginUriApi.ts
type LoginURI struct {
	URI   string      `json:"uri"`
	Match interface{} `json:"match,omitempty"`
}

// LoginData is the encrypted data for a cipher with the login type.
type LoginData struct {
	URIs     []LoginURI `json:"uris,omitempty"`
	Username string     `json:"username,omitempty"`
	Password string     `json:"password,omitempty"`
	RevDate  string     `json:"passwordRevisionDate,omitempty"`
	TOTP     string     `json:"totp,omitempty"`
}

// Field is used to store some additional fields.
type Field struct {
	// See https://github.com/bitwarden/jslib/blob/master/src/enums/fieldType.ts
	Type  int    `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// MapData is used for the data of secure note, card, and identity.
type MapData map[string]interface{}

// Cipher is an encrypted item that can be a login, a secure note, a card or an
// identity.
type Cipher struct {
	CouchID        string                 `json:"_id,omitempty"`
	CouchRev       string                 `json:"_rev,omitempty"`
	Type           CipherType             `json:"type"`
	SharedWithCozy bool                   `json:"shared_with_cozy"`
	Favorite       bool                   `json:"favorite,omitempty"`
	Name           string                 `json:"name"`
	Notes          string                 `json:"notes,omitempty"`
	FolderID       string                 `json:"folder_id,omitempty"`
	Login          *LoginData             `json:"login,omitempty"`
	Data           *MapData               `json:"data,omitempty"`
	Fields         []Field                `json:"fields"`
	Metadata       *metadata.CozyMetadata `json:"cozyMetadata,omitempty"`
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
		uris := make([]LoginURI, len(c.Login.URIs))
		copy(uris, c.Login.URIs)
		cloned.Login = &LoginData{
			URIs:     uris,
			Username: c.Login.Username,
			Password: c.Login.Password,
			RevDate:  c.Login.RevDate,
			TOTP:     c.Login.TOTP,
		}
	}
	cloned.Fields = make([]Field, len(c.Fields))
	copy(cloned.Fields, c.Fields)
	if c.Metadata != nil {
		cloned.Metadata = c.Metadata.Clone()
	}
	return &cloned
}

// SetID changes the cipher qualified identifier
func (c *Cipher) SetID(id string) { c.CouchID = id }

// SetRev changes the cipher revision
func (c *Cipher) SetRev(rev string) { c.CouchRev = rev }

// FindCiphersInFolder finds the ciphers in the given folder.
func FindCiphersInFolder(inst *instance.Instance, folderID string) ([]*Cipher, error) {
	var ciphers []*Cipher
	req := &couchdb.FindRequest{
		UseIndex: "by-folder-id",
		Selector: mango.Equal("folder_id", folderID),
	}
	err := couchdb.FindDocs(inst, consts.BitwardenCiphers, req, &ciphers)
	if err != nil {
		return nil, err
	}
	return ciphers, nil
}

var _ couchdb.Doc = &Cipher{}
