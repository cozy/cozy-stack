package sharings

import (
	"errors"
	"strings"

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
	SID         string `json:"_id,omitempty"`
	SRev        string `json:"_rev,omitempty"`
	SharingType string `json:"sharing_type"`

	// TODO check where it makes sense to use this flag
	// TODO use a date (RevokedAt *time.Time)?
	Revoked bool `json:"revoked,omitempty"`

	// Only one of Sharer or Recipients is filled
	// - Sharer is filled when Owner is false
	// - Recipients is filled when Owner is true
	Owner      bool     `json:"owner"`
	Sharer     Member   `json:"sharer,omitempty"`
	Recipients []Member `json:"recipients,omitempty"`

	Description string `json:"description,omitempty"`
	PreviewPath string `json:"preview_path,omitempty"`
	AppSlug     string `json:"app_slug"`

	// Just a cache for faster access to the permissions doc
	permissions *permissions.Permission
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

// Permissions returns the permissions doc for this sharing
func (s *Sharing) Permissions(db couchdb.Database) (*permissions.Permission, error) {
	var perms *permissions.Permission
	if s.permissions == nil {
		var err error
		if s.Owner {
			perms, err = permissions.GetForSharedByMe(db, s.SID)
		} else {
			perms, err = permissions.GetForSharedWithMe(db, s.SID)
		}
		if err != nil {
			return nil, err
		}
		s.permissions = perms
	}
	return s.permissions, nil
}

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

// GetMemberFromClientID returns the Recipient associated with the
// given clientID.
func (s *Sharing) GetMemberFromClientID(db couchdb.Database, clientID string) (*Member, error) {
	for _, m := range s.Recipients {
		if m.Client.ClientID == clientID {
			return &m, nil
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
				Type: contact.DocType(),
				ID:   contact.ID(),
			},
			contact: contact,
		}
		sharing.Recipients = append(sharing.Recipients, recipient)
	}
	if len(sharing.Recipients) == 0 {
		return nil, ErrRecipientDoesNotExist // TODO better error
	}

	// Create the permissions doc for previewing this sharing
	codes := make(map[string]string, len(sharing.Recipients))
	for _, recipient := range sharing.Recipients {
		var err error
		contactID := recipient.RefContact.ID
		codes[contactID], err = permissions.CreateCode(instance.OAuthSecret, instance.Domain, contactID)
		if err != nil {
			return nil, err
		}
	}

	if err := couchdb.CreateDoc(instance, sharing); err != nil {
		return nil, err
	}
	perms, err := permissions.CreateSharedByMeSet(instance, sharing.SID, codes, params.Permissions)
	if err != nil {
		return nil, err
	}
	sharing.permissions = perms
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
	sRec, err := sharing.GetMemberFromClientID(db, clientID)
	if err != nil {
		return nil, nil, err
	}
	if sRec == nil {
		return nil, nil, ErrRecipientDoesNotExist
	}
	return sharing, sRec, nil
}

// TODO i *instance.Instance vs db couchdb.Database on the whole pkg/sharings
// TODO add a comment
func GetSharingFromPermissions(db couchdb.Database, perms *permissions.Permission) (*Sharing, error) {
	parts := strings.SplitN(perms.SourceID, "/", 2)
	if len(parts) != 2 || parts[0] != consts.Sharings {
		return nil, errors.New("Invalid SourceID")
	}
	return FindSharing(db, parts[1])
}

var (
	_ couchdb.Doc = &Sharing{}
)
