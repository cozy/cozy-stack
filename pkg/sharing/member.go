package sharing

import (
	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/pkg/contacts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/permissions"
)

const (
	// MemberStatusOwner is the status for the member that is owner
	MemberStatusOwner = "owner"
	// MemberStatusMailNotSent is the initial status for a recipient, before
	// the mail invitation is sent
	MemberStatusMailNotSent = "mail-not-sent"
	// MemberStatusPendingInvitation is for a recipient that has not (yet)
	// accepted the sharing, but the invitation mail was sent
	MemberStatusPendingInvitation = "pending"
	// MemberStatusReady is for recipient that have accepted the sharing
	MemberStatusReady = "ready"
)

// Member contains the information about a recipient (or the sharer) for a sharing
type Member struct {
	Status   string `json:"status"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Instance string `json:"instance,omitempty"`
}

// Credentials is the struct with the secret stuff used for authentication &
// authorization.
type Credentials struct {
	// OAuth state to accept the sharing (authorize phase)
	State string `json:"state,omitempty"`

	// Information needed to send data to the member
	Client      *auth.Client      `json:"client,omitempty"`
	AccessToken *auth.AccessToken `json:"access_token,omitempty"`
}

// AddContact adds the contact with the given identifier
func (s *Sharing) AddContact(inst *instance.Instance, contactID string) error {
	c, err := contacts.Find(inst, contactID)
	if err != nil {
		return err
	}
	addr, err := c.ToMailAddress()
	if err != nil {
		return err
	}
	m := Member{
		Status:   MemberStatusMailNotSent,
		Name:     addr.Name,
		Email:    addr.Email,
		Instance: c.PrimaryCozyURL(),
	}
	s.Members = append(s.Members, m)
	state := crypto.Base64Encode(crypto.GenerateRandomBytes(StateLen))
	creds := Credentials{
		State: string(state),
	}
	s.Credentials = append(s.Credentials, creds)
	return nil
}

// FindMemberByState returns the member that is linked to the sharing by
// the given state
func (s *Sharing) FindMemberByState(db couchdb.Database, state string) (*Member, error) {
	if !s.Owner {
		return nil, ErrInvalidSharing
	}
	for i, c := range s.Credentials {
		if c.State == state {
			if len(s.Members) <= i+1 {
				return nil, ErrInvalidSharing
			}
			return &s.Members[i+1], nil
		}
	}
	return nil, ErrMemberNotFound
}

// FindMemberBySharecode returns the member that is linked to the sharing by
// the given sharecode
func (s *Sharing) FindMemberBySharecode(db couchdb.Database, sharecode string) (*Member, error) {
	if !s.Owner {
		return nil, ErrInvalidSharing
	}
	perms, err := permissions.GetForSharePreview(db, s.SID)
	if err != nil {
		return nil, err
	}
	var email string
	for e, code := range perms.Codes {
		if code == sharecode {
			email = e
			break
		}
	}
	for i, m := range s.Members {
		if m.Email == email {
			return &s.Members[i], nil
		}
	}
	return nil, ErrMemberNotFound
}
