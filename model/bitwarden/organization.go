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

// OrgMemberStatus is a type for the status of an organization member
type OrgMemberStatus int

const (
	// OrgMemberInvited is used when the member is invited but has not yet
	// accepted the invitation.
	OrgMemberInvited = 0
	// OrgMemberAccepted is used when the member is accepted but the owner has
	// not yet confirmed that the fingerprint is OK.
	OrgMemberAccepted = 1
	// OrgMemberConfirmed is used when the member is confirmed, and has access
	// to the organization key to decrypt/encrypt ciphers.
	OrgMemberConfirmed = 2
)

// OrgMember is a struct for describing a member of an organization.
type OrgMember struct {
	Email  string          `json:"email"` // me@<domain>
	Status OrgMemberStatus `json:"status"`
	Key    string          `json:"key,omitempty"`
}

// Organization is used to make collections of ciphers and can be used for
// sharing them with other users with cryptography mechanisms.
type Organization struct {
	CouchID  string                `json:"_id,omitempty"`
	CouchRev string                `json:"_rev,omitempty"`
	Name     string                `json:"name"`
	Members  map[string]OrgMember  `json:"members"` // the keys are the instances domains
	Metadata metadata.CozyMetadata `json:"cozyMetadata"`
}

// ID returns the organization identifier
func (o *Organization) ID() string { return o.CouchID }

// Rev returns the organization revision
func (o *Organization) Rev() string { return o.CouchRev }

// SetID changes the organization identifier
func (o *Organization) SetID(id string) { o.CouchID = id }

// SetRev changes the organization revision
func (o *Organization) SetRev(rev string) { o.CouchRev = rev }

// DocType returns the cipher document type
func (o *Organization) DocType() string { return consts.BitwardenOrganizations }

// Clone implements couchdb.Doc
func (o *Organization) Clone() couchdb.Doc {
	cloned := *o
	cloned.Members = make(map[string]OrgMember, len(o.Members))
	for k, v := range o.Members {
		cloned.Members[k] = v
	}
	return &cloned
}

// GetCozyOrganization returns the organization used to store the credentials
// for the konnectors running on the Cozy server.
func GetCozyOrganization(inst *instance.Instance, setting *settings.Settings) (*Organization, error) {
	if setting == nil || setting.PublicKey == "" {
		return nil, errors.New("No public key")
	}
	orgKey, err := setting.OrganizationKey()
	if err != nil {
		inst.Logger().WithField("nspace", "bitwarden").
			Infof("Cannot read the organization key: %s", err)
		return nil, err
	}
	key, err := crypto.EncryptWithRSA(setting.PublicKey, orgKey)
	if err != nil {
		inst.Logger().WithField("nspace", "bitwarden").
			Infof("Cannot encrypt with RSA: %s", err)
		return nil, err
	}

	email := inst.PassphraseSalt()
	org := Organization{
		CouchID: setting.OrganizationID,
		Name:    consts.BitwardenCozyOrganizationName,
		Members: map[string]OrgMember{
			inst.Domain: OrgMember{
				Email:  string(email),
				Key:    key,
				Status: OrgMemberConfirmed,
			},
		},
	}
	if setting.Metadata != nil {
		org.Metadata = *setting.Metadata
	}
	return &org, nil
}

var _ couchdb.Doc = &Organization{}
