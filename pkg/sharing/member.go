package sharing

import (
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/client/request"
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
	// MemberStatusRevoked is for a revoked member
	MemberStatusRevoked = "revoked"
)

// Member contains the information about a recipient (or the sharer) for a sharing
type Member struct {
	Status     string `json:"status"`
	Name       string `json:"name,omitempty"`
	PublicName string `json:"public_name,omitempty"`
	Email      string `json:"email"`
	Instance   string `json:"instance,omitempty"`
}

// Credentials is the struct with the secret stuff used for authentication &
// authorization.
type Credentials struct {
	// OAuth state to accept the sharing (authorize phase)
	State string `json:"state,omitempty"`

	// Information needed to send data to the member
	Client      *auth.Client      `json:"client,omitempty"`
	AccessToken *auth.AccessToken `json:"access_token,omitempty"`

	// XorKey is used to transform file identifiers
	XorKey []byte `json:"xor_key,omitempty"`

	// InboundClientID is the OAuth ClientID used for authentifying incoming
	// requests from the member
	InboundClientID string `json:"inbound_client_id,omitempty"`
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
		State:  string(state),
		XorKey: MakeXorKey(),
	}
	s.Credentials = append(s.Credentials, creds)
	return nil
}

// UpdateRecipients updates the list of recipients
func (s *Sharing) UpdateRecipients(inst *instance.Instance, members []Member) error {
	for i, m := range members {
		if m.Email != s.Members[i].Email {
			if contact, err := contacts.FindByEmail(inst, m.Email); err == nil {
				s.Members[i].Name = contact.PrimaryName()
			}
		}
		s.Members[i].Email = m.Email
		s.Members[i].PublicName = m.PublicName
		s.Members[i].Status = m.Status
		s.Members[i].Instance = m.Instance
	}
	return couchdb.UpdateDoc(inst, s)
}

// FindMemberByState returns the member that is linked to the sharing by
// the given state
func (s *Sharing) FindMemberByState(state string) (*Member, error) {
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

// FindMemberByInboundClientID returns the member that have used this client
// ID to make a request on the given sharing
func (s *Sharing) FindMemberByInboundClientID(clientID string) (*Member, error) {
	for i, c := range s.Credentials {
		if c.InboundClientID == clientID {
			return &s.Members[i+1], nil
		}
	}
	return nil, ErrMemberNotFound
}

// FindCredentials returns the credentials for the given member
func (s *Sharing) FindCredentials(m *Member) *Credentials {
	if s.Owner {
		for i, member := range s.Members {
			if i > 0 && *m == member {
				return &s.Credentials[i-1]
			}
		}
	} else {
		if *m == s.Members[0] {
			return &s.Credentials[0]
		}
	}
	return nil
}

// Refresh will refresh the access token, and persist the new access token in
// the sharing
func (c *Credentials) Refresh(inst *instance.Instance, s *Sharing, m *Member) error {
	u, err := url.Parse(m.Instance)
	if err != nil {
		return err
	}
	r := &auth.Request{
		Scheme: u.Scheme,
		Domain: u.Host,
	}
	token, err := r.RefreshToken(c.Client, c.AccessToken)
	if err != nil {
		return err
	}
	c.AccessToken.AccessToken = token.AccessToken
	return couchdb.UpdateDoc(inst, s)
}

// RevokeMember revoke the access granted to a member and contact it
func (s *Sharing) RevokeMember(inst *instance.Instance, m *Member, c *Credentials) error {
	// No need to contact the revoked member if the sharing is not ready
	if m.Status == MemberStatusReady {
		if err := s.NotifyMemberRevocation(inst, m, c); err != nil {
			inst.Logger().WithField("nspace", "sharing").
				Warnf("Error on revocation notification: %s", err)
		}

		if err := DeleteOAuthClient(inst, m, c); err != nil {
			return err
		}
	}
	m.Status = MemberStatusRevoked
	// Do not remove the credential to preserve the members / credentials order
	*c = Credentials{}

	return couchdb.UpdateDoc(inst, s)
}

// RevokeOwner revoke the access granted to the owner and notify it
func (s *Sharing) RevokeOwner(inst *instance.Instance) error {
	m := &s.Members[0]
	c := &s.Credentials[0]

	if err := s.NotifyMemberRevocation(inst, m, c); err != nil {
		inst.Logger().WithField("nspace", "sharing").
			Warnf("Error on revocation notification: %s", err)
	}
	if err := DeleteOAuthClient(inst, m, c); err != nil {
		return err
	}
	m.Status = MemberStatusRevoked
	s.Credentials = nil
	return couchdb.UpdateDoc(inst, s)
}

// NotifyMemberRevocation send a notification to this member that he/she was
// revoked from this sharing
func (s *Sharing) NotifyMemberRevocation(inst *instance.Instance, m *Member, c *Credentials) error {
	u, err := url.Parse(m.Instance)
	if m.Instance == "" || err != nil {
		return ErrInvalidSharing
	}
	var path string
	if m.Status == MemberStatusOwner {
		path = "/sharings/" + s.SID + "/answer"
	} else {
		path = "/sharings/" + s.SID
	}

	opts := &request.Options{
		Method: http.MethodDelete,
		Scheme: u.Scheme,
		Domain: u.Host,
		Path:   path,
		Headers: request.Headers{
			"Authorization": "Bearer " + c.AccessToken.AccessToken,
		},
	}
	res, err := request.Req(opts)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode/100 == 5 {
		return ErrInternalServerError
	}
	if res.StatusCode/100 == 4 {
		if res, err = RefreshToken(inst, s, m, c, opts, nil); err != nil {
			return err
		}
		res.Body.Close()
	}
	return nil
}
