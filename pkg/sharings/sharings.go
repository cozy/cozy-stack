package sharings

import (
	"errors"
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
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

	// Only one of Sharer or Recipients is filled
	// - Sharer is filled when Owner is false
	// - Recipients is filled when Owner is true
	Owner      bool     `json:"owner"`
	Sharer     *Member  `json:"sharer,omitempty"`
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
	cloned.Recipients = make([]Member, len(s.Recipients))
	for i := range s.Recipients {
		cloned.Recipients[i] = s.Recipients[i]
	}
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

// GetMemberFromClientID returns the Recipient associated with the
// given clientID.
func (s *Sharing) GetMemberFromClientID(db couchdb.Database, clientID string) (*Member, error) {
	for i := range s.Recipients {
		if s.Recipients[i].Client.ClientID == clientID {
			return &s.Recipients[i], nil
		}
	}
	return nil, ErrRecipientDoesNotExist
}

// GetMemberFromContactID returns the Member associated with the
// given contact ID.
func (s *Sharing) GetMemberFromContactID(db couchdb.Database, contactID string) (*Member, error) {
	for i := range s.Recipients {
		if s.Recipients[i].RefContact.ID == contactID {
			return &s.Recipients[i], nil
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
	}

	// Fetch the recipients in the database and populate Recipients
	for _, contactID := range params.Recipients {
		recipient := Member{
			Status: consts.SharingStatusPending,
			RefContact: couchdb.DocReference{
				Type: consts.Contacts,
				ID:   contactID,
			},
		}
		if contact := recipient.Contact(instance); contact == nil {
			continue
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
		codes[contactID], err = instance.CreateShareCode(contactID)
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
	res := &Sharing{}
	err := couchdb.GetDoc(db, consts.Sharings, sharingID, res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// GetSharingFromPermissions returns the sharing linked to the given permissions doc
func GetSharingFromPermissions(db couchdb.Database, perms *permissions.Permission) (*Sharing, error) {
	parts := strings.SplitN(perms.SourceID, "/", 2)
	if len(parts) != 2 || parts[0] != consts.Sharings {
		return nil, errors.New("Invalid SourceID")
	}
	return FindSharing(db, parts[1])
}

// AddRecipient adds a recipient identified by a contact_id to a sharing.
func (s *Sharing) AddRecipient(i *instance.Instance, contactID string) error {
	if !s.Owner {
		return ErrBadSharingType
	}

	// Add the recipient to the sharing document
	recipient := Member{
		Status: consts.SharingStatusPending,
		RefContact: couchdb.DocReference{
			Type: consts.Contacts,
			ID:   contactID,
		},
	}
	if contact := recipient.Contact(i); contact == nil {
		return ErrRecipientDoesNotExist
	}
	s.Recipients = append(s.Recipients, recipient)

	// Add a sharecode in the permissions document
	perms, err := s.Permissions(i)
	if err != nil {
		return err
	}
	if perms.Codes == nil {
		perms.Codes = make(map[string]string)
	}
	perms.Codes[contactID], err = i.CreateShareCode(contactID)
	if err != nil {
		return err
	}
	if err = couchdb.UpdateDoc(i, perms); err != nil {
		return err
	}

	return couchdb.UpdateDoc(i, s)
}

var (
	_ couchdb.Doc = &Sharing{}
)
