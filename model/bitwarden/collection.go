package bitwarden

import (
	"errors"

	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/metadata"
)

// Collection is used to regroup ciphers.
type Collection struct {
	CouchID        string                `json:"_id,omitempty"`
	CouchRev       string                `json:"_rev,omitempty"`
	OrganizationID string                `json:"organization_id"`
	Name           string                `json:"name"`
	Metadata       metadata.CozyMetadata `json:"cozyMetadata"`
}

// ID returns the collection identifier
func (c *Collection) ID() string { return c.CouchID }

// Rev returns the collection revision
func (c *Collection) Rev() string { return c.CouchRev }

// SetID changes the collection identifier
func (c *Collection) SetID(id string) { c.CouchID = id }

// SetRev changes the collection revision
func (c *Collection) SetRev(rev string) { c.CouchRev = rev }

// DocType returns the cipher document type
func (c *Collection) DocType() string { return consts.BitwardenCollections }

// Clone implements couchdb.Doc
func (c *Collection) Clone() couchdb.Doc {
	cloned := *c
	return &cloned
}

// Fetch implements permissions.Fetcher
func (c *Collection) Fetch(field string) []string {
	switch field {
	case "organization_id":
		return []string{c.OrganizationID}
	}
	return nil
}

// GetCozyCollection returns the collection used to store the credentials for
// the konnectors running on the Cozy server.
func GetCozyCollection(setting *settings.Settings) (*Collection, error) {
	orgKey, err := setting.OrganizationKey()
	if err != nil || len(orgKey) != 64 {
		return nil, errors.New("Missing organization key")
	}
	iv := crypto.GenerateRandomBytes(16)
	payload := []byte(consts.BitwardenCozyCollectionName)
	name, err := crypto.EncryptWithAES256HMAC(orgKey[:32], orgKey[32:], payload, iv)
	if err != nil {
		return nil, err
	}
	coll := Collection{
		CouchID:        setting.CollectionID,
		OrganizationID: setting.OrganizationID,
		Name:           name,
	}
	if setting.Metadata != nil {
		coll.Metadata = *setting.Metadata
	}
	return &coll, nil
}

// FindAllCollections returns all the collections, including the Cozy one.
func FindAllCollections(inst *instance.Instance, setting *settings.Settings) ([]*Collection, error) {
	var colls []*Collection
	req := &couchdb.AllDocsRequest{}
	if err := couchdb.GetAllDocs(inst, consts.BitwardenCollections, req, &colls); err != nil {
		if couchdb.IsNoDatabaseError(err) {
			_ = couchdb.CreateDB(inst, consts.BitwardenCollections)
		} else {
			return nil, err
		}
	}

	cozy, err := GetCozyCollection(setting)
	if err != nil {
		return nil, err
	}
	colls = append(colls, cozy)
	return colls, nil
}

var _ couchdb.Doc = &Collection{}
