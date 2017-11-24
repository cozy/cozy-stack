package sharings

import (
	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/contacts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/permissions"
)

// CreateSharingParams is filled from the request for creating a sharing.
type CreateSharingParams struct {
	SharingType string          `json:"sharing_type"`
	Permissions permissions.Set `json:"permissions"`
	Recipients  []string        `json:"recipients"`
	Description string          `json:"description,omitempty"`
	PreviewPath string          `json:"preview_path,omitempty"`
}

// Sharing contains all the information about a sharing.
type Sharing struct {
	SID         string          `json:"_id,omitempty"`
	SRev        string          `json:"_rev,omitempty"`
	SharingType string          `json:"sharing_type"`
	Permissions permissions.Set `json:"permissions,omitempty"`
	Sharer      Member          `json:"sharer,omitempty"`
	Recipients  []Member        `json:"recipients,omitempty"`
	Description string          `json:"description,omitempty"`
	PreviewPath string          `json:"preview_path,omitempty"`
	AppSlug     string          `json:"app_slug"`
	Owner       bool            `json:"owner"`
	Revoked     bool            `json:"revoked,omitempty"`
}

// ID returns the sharing qualified identifier
func (s *Sharing) ID() string { return s.SID }

// Rev returns the sharing revision
func (s *Sharing) Rev() string { return s.SRev }

// DocType returns the sharing document type
func (s *Sharing) DocType() string { return consts.Sharings }

// Clone implements couchdb.Doc
func (s *Sharing) Clone() couchdb.Doc {
	cloned := *s
	return &cloned
}

// SetID changes the sharing qualified identifier
func (s *Sharing) SetID(id string) { s.SID = id }

// SetRev changes the sharing revision
func (s *Sharing) SetRev(rev string) { s.SRev = rev }

// Contacts returns the sharing recipients
// TODO see how this method is used to try to find something better
func (s *Sharing) Contacts(db couchdb.Database) ([]*contacts.Contact, error) {
	var recipients []*contacts.Contact

	for _, rec := range s.Recipients {
		recipient, err := GetContact(db, rec.RefContact.ID)
		if err != nil {
			return nil, err
		}
		rec.contact = recipient
		recipients = append(recipients, recipient)
	}

	return recipients, nil
}

// GetSharingRecipientFromClientID returns the Recipient associated with the
// given clientID.
func (s *Sharing) GetSharingRecipientFromClientID(db couchdb.Database, clientID string) (*Member, error) {
	for _, recStatus := range s.Recipients {
		if recStatus.Client.ClientID == clientID {
			return &recStatus, nil
		}
	}
	return nil, ErrRecipientDoesNotExist
}

// GetMemberFromRecipientID returns the Member associated with the
// given recipient ID.
// TODO refactor
func (s *Sharing) GetMemberFromRecipientID(db couchdb.Database, recID string) (*Member, error) {
	for _, recStatus := range s.Recipients {
		if recStatus.contact == nil {
			r, err := GetContact(db, recStatus.RefContact.ID)
			if err != nil {
				return nil, err
			}
			recStatus.contact = r
		}
		if recStatus.contact.ID() == recID {
			return &recStatus, nil
		}
	}
	return nil, ErrRecipientDoesNotExist
}

// CheckSharingType returns an error if the sharing type is incorrect
func CheckSharingType(sharingType string) error {
	switch sharingType {
	case consts.OneShotSharing, consts.OneWaySharing, consts.TwoWaySharing:
		return nil
	}
	return ErrBadSharingType
}

// CreateSharing checks the sharing, creates the document in
// base and starts the sharing process by registering the sharer at each
// recipient as a new OAuth client.
func CreateSharing(instance *instance.Instance, params *CreateSharingParams, slug string) (*Sharing, error) {
	sharingType := params.SharingType
	if err := CheckSharingType(sharingType); err != nil {
		return nil, err
	}

	sharing := &Sharing{
		SharingType: sharingType,
		Permissions: params.Permissions,
		Recipients:  make([]Member, 0, len(params.Recipients)),
		Description: params.Description,
		PreviewPath: params.PreviewPath,
		AppSlug:     slug,
		Owner:       true,
		Revoked:     false,
	}

	// Fetch the recipients in the database and populate Recipients
	for _, contactID := range params.Recipients {
		contact, err := GetContact(instance, contactID)
		if err != nil {
			continue
		}
		recipient := Member{
			Status: consts.SharingStatusPending,
			RefContact: couchdb.DocReference{
				Type: consts.Contacts,
				ID:   contact.DocID,
			},
			contact: contact,
		}
		sharing.Recipients = append(sharing.Recipients, recipient)
	}
	if len(sharing.Recipients) == 0 {
		return nil, ErrRecipientDoesNotExist // TODO better error
	}

	if err := couchdb.CreateDoc(instance, sharing); err != nil {
		return nil, err
	}
	return sharing, nil
}

// FindSharing retrieves a sharing document from its ID
func FindSharing(db couchdb.Database, sharingID string) (*Sharing, error) {
	var res *Sharing
	err := couchdb.GetDoc(db, consts.Sharings, sharingID, res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// FindSharingRecipient retrieve a sharing recipient from its clientID and sharingID
// TODO see how this method is used to try to find a better name (or refactor)
func FindSharingRecipient(db couchdb.Database, sharingID, clientID string) (*Sharing, *Member, error) {
	sharing, err := FindSharing(db, sharingID)
	if err != nil {
		return nil, nil, err
	}
	sRec, err := sharing.GetSharingRecipientFromClientID(db, clientID)
	if err != nil {
		return nil, nil, err
	}
	if sRec == nil {
		return nil, nil, ErrRecipientDoesNotExist
	}
	return sharing, sRec, nil
}

// RecipientInfo describes the recipient information that will be transmitted to
// the sharing workers.
type RecipientInfo struct {
	URL         string
	Scheme      string
	Client      auth.Client
	AccessToken auth.AccessToken
}

// ExtractRecipientInfo returns a RecipientInfo from a Member
func ExtractRecipientInfo(db couchdb.Database, rec *Member) (*RecipientInfo, error) {
	recipient, err := GetContact(db, rec.RefContact.ID)
	if err != nil {
		return nil, err
	}
	u, scheme, err := ExtractDomainAndScheme(recipient)
	if err != nil {
		return nil, err
	}
	info := &RecipientInfo{
		URL:         u,
		Scheme:      scheme,
		AccessToken: rec.AccessToken,
		Client:      rec.Client,
	}
	return info, nil
}

var (
	_ couchdb.Doc = &Sharing{}
)
