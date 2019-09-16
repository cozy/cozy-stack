package settings

import (
	"errors"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
)

// Settings is the struct that holds the birwarden settings
type Settings struct {
	CouchRev                string `json:"_rev,omitempty"`
	PassphraseKdf           int    `json:"passphrase_kdf,omitempty"`
	PassphraseKdfIterations int    `json:"passphrase_kdf_iterations,omitempty"`
	PassphraseHint          string `json:"passphrase_hint,omitempty"`
	SecurityStamp           string `json:"security_stamp,omitempty"`
	Key                     string `json:"key,omitempty"`
	PublicKey               string `json:"public_key,omitempty"`
	PrivateKey              string `json:"private_key,omitempty"`
	OrganizationKey         []byte `json:"organization_key,omitempty"`
	OrganizationID          string `json:"organization_id,omitempty"`
	CollectionID            string `json:"collection_id,omitempty"`
}

// ID returns the settings qualified identifier
func (s *Settings) ID() string { return consts.BitwardenSettingsID }

// Rev returns the settings revision
func (s *Settings) Rev() string { return s.CouchRev }

// DocType returns the settings document type
func (s *Settings) DocType() string { return consts.Settings }

// Clone implements couchdb.Doc
func (s *Settings) Clone() couchdb.Doc {
	cloned := *s
	return &cloned
}

// SetID changes the settings qualified identifier
func (s *Settings) SetID(id string) { panic(errors.New("unsupported")) }

// SetRev changes the settings revision
func (s *Settings) SetRev(rev string) { s.CouchRev = rev }

// Save persists the settings document for bitwarden in CouchDB.
func (s *Settings) Save(inst *instance.Instance) error {
	if s.CouchRev == "" {
		return couchdb.CreateNamedDocWithDB(inst, s)
	}
	return couchdb.UpdateDoc(inst, s)
}

// SetKeyPair is used to save the key pair of the user, that will be used to
// share passwords with the cozy organization.
func (s *Settings) SetKeyPair(inst *instance.Instance, pub, priv string) error {
	s.PublicKey = pub
	s.PrivateKey = priv
	if len(s.OrganizationKey) != 64 {
		s.OrganizationKey = crypto.GenerateRandomBytes(64)
	}
	var err error
	if s.OrganizationID == "" {
		s.OrganizationID, err = couchdb.UUID(inst)
		if err != nil {
			return err
		}
	}
	if s.CollectionID == "" {
		s.CollectionID, err = couchdb.UUID(inst)
		if err != nil {
			return err
		}
	}
	return couchdb.UpdateDoc(inst, s)
}

// Get returns the settings document for bitwarden.
func Get(inst *instance.Instance) (*Settings, error) {
	settings := &Settings{}
	err := couchdb.GetDoc(inst, consts.Settings, consts.BitwardenSettingsID, settings)
	if err != nil && !couchdb.IsNotFoundError(err) {
		return nil, err
	}
	return settings, nil
}

var _ couchdb.Doc = &Settings{}
