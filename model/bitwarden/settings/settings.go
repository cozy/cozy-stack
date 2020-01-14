package settings

import (
	"encoding/base64"
	"errors"

	"github.com/cozy/cozy-stack/model/account"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/metadata"
)

// DocTypeVersion represents the doctype version. Each time this document
// structure is modified, update this value
const DocTypeVersion = "1"

// ErrMissingOrgKey is used when the organization key does not exist
var ErrMissingOrgKey = errors.New("No organization key")

// Settings is the struct that holds the birwarden settings
type Settings struct {
	CouchRev                string                 `json:"_rev,omitempty"`
	PassphraseKdf           int                    `json:"passphrase_kdf,omitempty"`
	PassphraseKdfIterations int                    `json:"passphrase_kdf_iterations,omitempty"`
	PassphraseHint          string                 `json:"passphrase_hint,omitempty"`
	SecurityStamp           string                 `json:"security_stamp,omitempty"`
	Key                     string                 `json:"key,omitempty"`
	PublicKey               string                 `json:"public_key,omitempty"`
	PrivateKey              string                 `json:"private_key,omitempty"`
	EncryptedOrgKey         string                 `json:"encrypted_organization_key,omitempty"`
	OrganizationID          string                 `json:"organization_id,omitempty"`
	CollectionID            string                 `json:"collection_id,omitempty"`
	EquivalentDomains       [][]string             `json:"equivalent_domains,omitempty"`
	GlobalEquivalentDomains []int                  `json:"global_equivalent_domains,omitempty"`
	Metadata                *metadata.CozyMetadata `json:"cozyMetadata,omitempty"`
	ExtensionInstalled      bool                   `json:"extension_installed,omitempty"`
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
	if s.Metadata != nil {
		cloned.Metadata = s.Metadata.Clone()
	}
	return &cloned
}

// SetID changes the settings qualified identifier
func (s *Settings) SetID(id string) { panic(errors.New("unsupported")) }

// SetRev changes the settings revision
func (s *Settings) SetRev(rev string) { s.CouchRev = rev }

// Save persists the settings document for bitwarden in CouchDB.
func (s *Settings) Save(inst *instance.Instance) error {
	if s.Metadata == nil {
		md := metadata.New()
		md.DocTypeVersion = DocTypeVersion
		s.Metadata = md
	} else {
		s.Metadata.ChangeUpdatedAt()
	}
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
	if err := s.EnsureCozyOrganization(inst); err != nil {
		return err
	}
	return couchdb.UpdateDoc(inst, s)
}

// EnsureCozyOrganization make sure that the settings for the Cozy organization
// are set.
func (s *Settings) EnsureCozyOrganization(inst *instance.Instance) error {
	var err error
	if len(s.EncryptedOrgKey) == 0 {
		orgKey := crypto.GenerateRandomBytes(64)
		b64 := base64.StdEncoding.EncodeToString(orgKey)
		s.EncryptedOrgKey, err = account.EncryptCredentialsData(b64)
		if err != nil {
			return err
		}
	}
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
	return nil
}

// OrganizationKey returns the organization key (in clear, not encrypted).
func (s *Settings) OrganizationKey() ([]byte, error) {
	if len(s.EncryptedOrgKey) == 0 {
		return nil, ErrMissingOrgKey
	}
	decrypted, err := account.DecryptCredentialsData(s.EncryptedOrgKey)
	if err != nil {
		return nil, err
	}
	b64, ok := decrypted.(string)
	if !ok {
		return nil, errors.New("Invalid key")
	}
	return base64.StdEncoding.DecodeString(b64)
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

// UpdateRevisionDate updates the updatedAt field of the bitwarden settings
// document. This field is used to know by some clients to know the date of the
// last change on the server before doing a full sync.
func UpdateRevisionDate(inst *instance.Instance, settings *Settings) {
	var err error
	if settings == nil {
		settings, err = Get(inst)
	}
	if err == nil {
		err = settings.Save(inst)
	}
	if err != nil {
		inst.Logger().WithField("nspace", "bitwarden").
			Infof("Cannot update revision date: %s", err)
	}
}

var _ couchdb.Doc = &Settings{}
