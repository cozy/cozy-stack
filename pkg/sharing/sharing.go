package sharing

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/cozy/cozy-stack/client/auth"
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/contacts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/crypto"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/web/jsonapi"
)

const (
	// StateLen is the number of bytes for the OAuth state parameter
	StateLen = 16
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

	// The OAuth ClientID used for authentifying incoming requests from the member
	InboundClientID string `json:"inbound_client_id,omitempty"`
}

// Rule describes how the sharing behave when a document matching the rule is
// added, updated or deleted.
type Rule struct {
	Title    string   `json:"title"`
	DocType  string   `json:"doctype"`
	Selector string   `json:"selector,omitempty"`
	Values   []string `json:"values"`
	Local    bool     `json:"local,omitempty"`
	Add      string   `json:"add"`
	Update   string   `json:"update"`
	Remove   string   `json:"remove"`
}

// Sharing contains all the information about a sharing.
type Sharing struct {
	SID  string `json:"_id,omitempty"`
	SRev string `json:"_rev,omitempty"`

	Active      bool      `json:"active,omitempty"`
	Owner       bool      `json:"owner,omitempty"`
	Open        bool      `json:"open_sharing,omitempty"`
	Description string    `json:"description,omitempty"`
	AppSlug     string    `json:"app_slug"`
	PreviewPath string    `json:"preview_path,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	Rules []Rule `json:"rules"`

	// Members[0] is the owner, Members[1...] are the recipients
	Members []Member `json:"members"`

	// On the owner, credentials[i] is associated to members[i+1]
	// On a recipient, there is only credentials[0] (for the owner)
	Credentials []Credentials `json:"credentials,omitempty"`
}

// ID returns the sharing qualified identifier
func (s *Sharing) ID() string { return s.SID }

// Rev returns the sharing revision
func (s *Sharing) Rev() string { return s.SRev }

// DocType returns the sharing document type
func (s *Sharing) DocType() string { return consts.Sharings }

// SetID changes the sharing qualified identifier
func (s *Sharing) SetID(id string) { s.SID = id }

// SetRev changes the sharing revision
func (s *Sharing) SetRev(rev string) { s.SRev = rev }

// Clone implements couchdb.Doc
func (s *Sharing) Clone() couchdb.Doc {
	cloned := *s
	cloned.Members = make([]Member, len(s.Members))
	for i := range s.Members {
		cloned.Members[i] = s.Members[i]
	}
	cloned.Credentials = make([]Credentials, len(s.Credentials))
	for i := range s.Credentials {
		cloned.Credentials[i] = s.Credentials[i]
	}
	cloned.Rules = make([]Rule, len(s.Rules))
	for i := range s.Rules {
		cloned.Rules[i] = s.Rules[i]
	}
	return &cloned
}

// ReadOnly returns true only if the rules forbid that a change on the
// recipients' cozy instances can be propagated to the sharer's cozy.
func (s *Sharing) ReadOnly() bool {
	for _, rule := range s.Rules {
		if rule.Add == "sync" || rule.Update == "sync" || rule.Remove == "sync" {
			return false
		}
	}
	return true
}

// BeOwner is a function that setup a sharing on the cozy of its owner
func (s *Sharing) BeOwner(inst *instance.Instance, slug string) error {
	s.Active = true
	s.Owner = true
	if s.AppSlug == "" {
		s.AppSlug = slug
	}
	if s.AppSlug == "" {
		s.PreviewPath = ""
	}
	s.CreatedAt = time.Now()
	s.UpdatedAt = s.CreatedAt

	name, err := inst.PublicName()
	if err != nil {
		return err
	}
	email, err := inst.SettingsEMail()
	if err != nil {
		return err
	}

	s.Members = make([]Member, 1)
	s.Members[0].Status = MemberStatusOwner
	s.Members[0].Name = name
	s.Members[0].Email = email
	s.Members[0].Instance = inst.PageURL("", nil)

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
		State: string(state),
	}
	s.Credentials = append(s.Credentials, creds)
	return nil
}

// CreatePreviewPermissions creates the permissions doc for previewing this sharing
func (s *Sharing) CreatePreviewPermissions(inst *instance.Instance) (map[string]string, error) {
	codes := make(map[string]string, len(s.Members)-1)
	for i, m := range s.Members {
		if i == 0 {
			continue
		}
		var err error
		codes[m.Email], err = inst.CreateShareCode(m.Email)
		if err != nil {
			return nil, err
		}
	}

	set := make(permissions.Set, len(s.Rules))
	getVerb := permissions.VerbSplit("GET")
	for i, rule := range s.Rules {
		set[i] = permissions.Rule{
			Type:     rule.DocType,
			Title:    rule.Title,
			Verbs:    getVerb,
			Selector: rule.Selector,
			Values:   rule.Values,
		}
	}

	_, err := permissions.CreateSharePreviewSet(inst, s.SID, codes, set)
	if err != nil {
		return nil, err
	}
	return codes, nil
}

// Create checks that the sharing is OK and it persists it in CouchDB if it is the case.
func (s *Sharing) Create(inst *instance.Instance) (map[string]string, error) {
	// TODO validate the doctype of each rule
	if len(s.Rules) == 0 {
		return nil, ErrNoRules
	}
	if len(s.Members) < 2 {
		return nil, ErrNoRecipients
	}

	if err := couchdb.CreateDoc(inst, s); err != nil {
		return nil, err
	}

	if s.Owner && s.PreviewPath != "" {
		return s.CreatePreviewPermissions(inst)
	}
	return nil, nil
}

// CreateRequest prepares a sharing as just a request that the user will have to
// accept before it does anything.
func (s *Sharing) CreateRequest(inst *instance.Instance) error {
	// TODO validate the doctype of each rule
	if len(s.Rules) == 0 {
		return ErrNoRules
	}
	if len(s.Members) < 2 {
		return ErrNoRecipients
	}
	// TODO check members

	s.Active = false
	s.Owner = false
	s.UpdatedAt = time.Now()
	s.Credentials = make([]Credentials, 1)

	return couchdb.CreateNamedDoc(inst, s)
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

// APISharing is used to serialize a Sharing to JSON-API
type APISharing struct {
	*Sharing
	// XXX Hide the credentials
	Credentials *interface{} `json:"credentials,omitempty"`
}

// Included is part of jsonapi.Object interface
func (s *APISharing) Included() []jsonapi.Object { return nil }

// Relationships is part of jsonapi.Object interface
func (s *APISharing) Relationships() jsonapi.RelationshipMap { return nil }

// Links is part of jsonapi.Object interface
func (s *APISharing) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/sharings/" + s.SID}
}

var _ jsonapi.Object = (*APISharing)(nil)

// RegisterClient asks the Cozy of the member to register a new OAuth client
func (m *Member) RegisterClient(inst *instance.Instance, u *url.URL) (*auth.Client, error) {
	req := &auth.Request{
		Domain: u.Host,
		Scheme: u.Scheme,
	}

	publicName, _ := inst.PublicName()
	if publicName == "" {
		publicName = inst.Domain
	}
	redirectURI := inst.PageURL("/sharings/answer", nil)
	clientURI := inst.PageURL("", nil)
	authClient := &auth.Client{
		RedirectURIs: []string{redirectURI},
		ClientName:   publicName,
		ClientKind:   "sharing",
		SoftwareID:   "github.com/cozy/cozy-stack",
		ClientURI:    clientURI,
	}

	resClient, err := req.RegisterClient(authClient)
	if err != nil {
		return nil, err
	}
	m.Instance = u.String()
	return resClient, nil
}

// CreateSharingRequest sends information about the sharing to the recipient's cozy
func (m *Member) CreateSharingRequest(inst *instance.Instance, s *Sharing, u *url.URL) error {
	// TODO translate ids of files/folders in the rules sent to the recipients
	sh := APISharing{
		&Sharing{
			SID:         s.SID,
			Active:      false,
			Owner:       false,
			Open:        s.Open,
			Description: s.Description,
			AppSlug:     s.AppSlug,
			PreviewPath: s.PreviewPath,
			CreatedAt:   s.CreatedAt,
			UpdatedAt:   s.UpdatedAt,
			Rules:       s.Rules,
			Members:     s.Members,
		},
		nil,
	}
	data, err := jsonapi.MarshalObject(&sh)
	if err != nil {
		return err
	}
	body, err := json.Marshal(jsonapi.Document{Data: &data})
	if err != nil {
		return err
	}
	res, err := request.Req(&request.Options{
		Method: http.MethodPut,
		Scheme: u.Scheme,
		Domain: u.Host,
		Path:   "/sharings/" + s.SID,
		Headers: request.Headers{
			"Accept":       "application/vnd.api+json",
			"Content-Type": "application/vnd.api+json",
		},
		Body: bytes.NewReader(body),
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode/100 != 2 {
		return ErrRequestFailed
	}

	return nil
}

// RegisterCozyURL saves a new Cozy URL for a member
func (s *Sharing) RegisterCozyURL(inst *instance.Instance, m *Member, u *url.URL) error {
	if u.Host == "" {
		return ErrInvalidURL
	}
	if u.Scheme == "" {
		u.Scheme = "https" // Set https as the default scheme
	}
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""

	if !s.Owner {
		return ErrInvalidSharing
	}
	if len(s.Members) != len(s.Credentials)+1 {
		return ErrInvalidSharing
	}
	var creds *Credentials
	for i, member := range s.Members {
		if *m == member {
			creds = &s.Credentials[i-1]
		}
	}
	if creds == nil {
		return ErrInvalidSharing
	}

	client, err := m.RegisterClient(inst, u)
	if err != nil {
		logger.WithDomain(inst.Domain).Warnf("[sharing] Error on OAuth client registration: %s", err)
		return ErrInvalidURL
	}
	creds.Client = client

	if err = m.CreateSharingRequest(inst, s, u); err != nil {
		logger.WithDomain(inst.Domain).Warnf("[sharing] Error on sharing request: %s", err)
		return ErrRequestFailed
	}
	return couchdb.UpdateDoc(inst, s)
}

// GenerateOAuthURL takes care of creating a correct OAuth request for
// the given member of the sharing.
func (m *Member) GenerateOAuthURL(s *Sharing) (string, error) {
	if !s.Owner {
		return "", ErrInvalidSharing
	}
	if len(s.Members) != len(s.Credentials)+1 {
		return "", ErrInvalidSharing
	}
	var creds *Credentials
	for i, member := range s.Members {
		if *m == member {
			creds = &s.Credentials[i-1]
		}
	}
	if creds == nil {
		return "", ErrInvalidSharing
	}
	if m.Instance == "" || creds.Client.ClientID == "" {
		return "", ErrNoOAuthClient
	}

	u, err := url.Parse(m.Instance)
	if err != nil {
		return "", err
	}
	u.Path = "/auth/authorize/sharing"

	q := url.Values{
		"sharing_id": {s.SID},
		"client_id":  {creds.Client.ClientID},
		"state":      {creds.State},
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

var _ couchdb.Doc = &Sharing{}
