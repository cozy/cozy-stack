package sharing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/contacts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/web/jsonapi"
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

// PrimaryName returns the main name of this member
func (m *Member) PrimaryName() string {
	if m.Name != "" {
		return m.Name
	}
	if m.PublicName != "" {
		return m.PublicName
	}
	return m.Email
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

// AddContacts adds a list of contacts on the sharer cozy
func (s *Sharing) AddContacts(inst *instance.Instance, contactIDs []string) error {
	for _, id := range contactIDs {
		if err := s.AddContact(inst, id); err != nil {
			return err
		}
	}
	var err error
	var codes map[string]string
	if s.PreviewPath != "" {
		if codes, err = s.CreatePreviewPermissions(inst); err != nil {
			return err
		}
	}
	if err = s.SendMails(inst, codes); err != nil {
		return err
	}
	cloned := s.Clone().(*Sharing)
	go cloned.NotifyRecipients(inst, nil)
	return nil
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

// APIDelegateAddContacts is used to serialize a request to add contacts to
// JSON-API
type APIDelegateAddContacts struct {
	sid     string
	members []Member
}

// ID returns the sharing qualified identifier
func (a *APIDelegateAddContacts) ID() string { return a.sid }

// Rev returns the sharing revision
func (a *APIDelegateAddContacts) Rev() string { return "" }

// DocType returns the sharing document type
func (a *APIDelegateAddContacts) DocType() string { return consts.Sharings }

// SetID changes the sharing qualified identifier
func (a *APIDelegateAddContacts) SetID(id string) {}

// SetRev changes the sharing revision
func (a *APIDelegateAddContacts) SetRev(rev string) {}

// Clone is part of jsonapi.Object interface
func (a *APIDelegateAddContacts) Clone() couchdb.Doc {
	panic("APIDelegateAddContacts must not be cloned")
}

// Links is part of jsonapi.Object interface
func (a *APIDelegateAddContacts) Links() *jsonapi.LinksList { return nil }

// Included is part of jsonapi.Object interface
func (a *APIDelegateAddContacts) Included() []jsonapi.Object { return nil }

// Relationships is part of jsonapi.Object interface
func (a *APIDelegateAddContacts) Relationships() jsonapi.RelationshipMap {
	return jsonapi.RelationshipMap{
		"recipients": jsonapi.Relationship{
			Data: a.members,
		},
	}
}

var _ jsonapi.Object = (*APIDelegateAddContacts)(nil)

// DelegateAddContacts adds a list of contacts on a recipient cozy. Part of
// the work is delegated to owner cozy, but the invitation mail is still sent
// from the recipient cozy.
func (s *Sharing) DelegateAddContacts(inst *instance.Instance, contactIDs []string) error {
	api := &APIDelegateAddContacts{}
	api.sid = s.SID
	for _, id := range contactIDs {
		c, err := contacts.Find(inst, id)
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
		api.members = append(api.members, m)
	}
	data, err := jsonapi.MarshalObject(api)
	if err != nil {
		return err
	}
	body, err := json.Marshal(jsonapi.Document{Data: &data})
	if err != nil {
		return err
	}
	u, err := url.Parse(s.Members[0].Instance)
	if err != nil {
		return err
	}
	c := &s.Credentials[0]
	opts := &request.Options{
		Method: http.MethodPost,
		Scheme: u.Scheme,
		Domain: u.Host,
		Path:   "/sharings/" + s.SID + "/recipients/delegated",
		Headers: request.Headers{
			"Accept":        "application/json",
			"Content-Type":  "application/vnd.api+json",
			"Authorization": "Bearer " + c.AccessToken.AccessToken,
		},
		Body: bytes.NewReader(body),
	}
	res, err := request.Req(opts)
	if err != nil {
		return err
	}
	if res.StatusCode/100 == 4 {
		res.Body.Close()
		if res, err = RefreshToken(inst, s, &s.Members[0], c, opts, body); err != nil {
			return err
		}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ErrInternalServerError
	}
	var states map[string]string
	if err = json.NewDecoder(res.Body).Decode(&states); err != nil {
		return err
	}
	s.Members = append(s.Members, api.members...)
	if err := couchdb.UpdateDoc(inst, s); err != nil {
		return err
	}
	return s.SendMailsToMembers(inst, api.members, states)
}

// AddDelegatedContact adds a contact on the owner cozy, but for a contact from
// a recipient (open_sharing: true only)
func (s *Sharing) AddDelegatedContact(inst *instance.Instance, email string) string {
	m := Member{
		Status: MemberStatusPendingInvitation,
		Email:  email,
	}
	s.Members = append(s.Members, m)
	state := crypto.Base64Encode(crypto.GenerateRandomBytes(StateLen))
	creds := Credentials{
		State:  string(state),
		XorKey: MakeXorKey(),
	}
	s.Credentials = append(s.Credentials, creds)
	return creds.State
}

// DelegateDiscovery delegates the POST discovery when a recipient has invited
// another person to a sharing, and this person accepts the sharing on the
// recipient cozy. The calls is delegated to the owner cozy.
func (s *Sharing) DelegateDiscovery(inst *instance.Instance, state, cozyURL string) (string, error) {
	u, err := url.Parse(s.Members[0].Instance)
	if err != nil {
		return "", err
	}
	v := url.Values{}
	v.Add("state", state)
	v.Add("url", cozyURL)
	body := []byte(v.Encode())
	c := &s.Credentials[0]
	opts := &request.Options{
		Method: http.MethodPut,
		Scheme: u.Scheme,
		Domain: u.Host,
		Path:   "/sharings/" + s.SID + "/discovery",
		Headers: request.Headers{
			"Accept":        "application/json",
			"Content-Type":  "application/x-www-form-urlencoded",
			"Authorization": "Bearer " + c.AccessToken.AccessToken,
		},
		Body: bytes.NewReader(body),
	}
	res, err := request.Req(opts)
	if err != nil {
		return "", err
	}
	if res.StatusCode/100 == 4 {
		res.Body.Close()
		if res, err = RefreshToken(inst, s, &s.Members[0], c, opts, body); err != nil {
			return "", err
		}
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusBadRequest {
		return "", ErrInvalidURL
	}
	if res.StatusCode/100 != 2 {
		return "", ErrInternalServerError
	}
	var success map[string]string
	if err = json.NewDecoder(res.Body).Decode(&success); err != nil {
		return "", err
	}
	return success["redirect"], nil
}

// UpdateRecipients updates the list of recipients
func (s *Sharing) UpdateRecipients(inst *instance.Instance, members []Member) error {
	for i, m := range members {
		if i >= len(s.Members) {
			s.Members = append(s.Members, Member{})
		}
		if m.Email != s.Members[i].Email {
			if contact, err := contacts.FindByEmail(inst, m.Email); err == nil {
				s.Members[i].Name = contact.PrimaryName()
			}
		}
		s.Members[i].Email = m.Email
		s.Members[i].PublicName = m.PublicName
		s.Members[i].Status = m.Status
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
func (s *Sharing) FindMemberBySharecode(db prefixer.Prefixer, sharecode string) (*Member, error) {
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

// NotifyRecipients will push the updated list of members of the sharing to the
// active recipients. It is meant to be used in a goroutine, errors are just
// logged (nothing critical here).
func (s *Sharing) NotifyRecipients(inst *instance.Instance, except *Member) {
	if !s.Owner {
		return
	}
	if len(s.Members) != len(s.Credentials)+1 {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			var err error
			switch r := r.(type) {
			case error:
				err = r
			default:
				err = fmt.Errorf("%v", r)
			}
			stack := make([]byte, 4<<10) // 4 KB
			length := runtime.Stack(stack, false)
			log := inst.Logger().WithField("panic", true).WithField("nspace", "sharing")
			log.Errorf("PANIC RECOVER %s: %s", err.Error(), stack[:length])
		}
	}()

	// XXX Wait a bit to avoid pressure on recipients cozy after delegated operations
	time.Sleep(3 * time.Second)

	active := false
	for i, m := range s.Members {
		if i > 0 && m.Status == MemberStatusReady && &s.Members[i] != except {
			active = true
			break
		}
	}
	if !active {
		return
	}

	var members struct {
		Members []Member `json:"data"`
	}
	members.Members = make([]Member, len(s.Members))
	for i, m := range s.Members {
		members.Members[i] = Member{
			Status:     m.Status,
			PublicName: m.PublicName,
			Email:      m.Email,
			// Instance and name are private
		}
	}
	body, err := json.Marshal(members)
	if err != nil {
		inst.Logger().WithField("nspace", "sharing").
			Warnf("Can't serialize the updated members list for %s: %s", s.SID, err)
		return
	}

	for i, m := range s.Members {
		if i == 0 || m.Status != MemberStatusReady || &s.Members[i] == except {
			continue
		}
		u, err := url.Parse(m.Instance)
		if m.Instance == "" || err != nil {
			inst.Logger().WithField("nspace", "sharing").
				Infof("Invalid instance URL %s: %s", m.Instance, err)
			continue
		}
		c := &s.Credentials[i-1]
		opts := &request.Options{
			Method: http.MethodPut,
			Scheme: u.Scheme,
			Domain: u.Host,
			Path:   "/sharings/" + s.SID + "/recipients",
			Headers: request.Headers{
				"Accept":        "application/vnd.api+json",
				"Content-Type":  "application/vnd.api+json",
				"Authorization": "Bearer " + c.AccessToken.AccessToken,
			},
			Body: bytes.NewReader(body),
		}
		res, err := request.Req(opts)
		if err != nil {
			inst.Logger().WithField("nspace", "sharing").
				Infof("Can't notify %#v about the updated members list: %s", m, err)
			continue
		}
		res.Body.Close()
		if res.StatusCode/100 == 4 {
			if res, err = RefreshToken(inst, s, &s.Members[i], c, opts, body); err != nil {
				inst.Logger().WithField("nspace", "sharing").
					Infof("Can't notify %#v about the updated members list: %s", m, err)
				continue
			}
			res.Body.Close()
		}
	}
}
