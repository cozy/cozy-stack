package sharing

import (
	"errors"
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/contacts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
)

const (
	// StatusOwner is the status for the member that is owner
	StatusOwner = "owner"
	// StatusMailNotSent is the initial status for a recipient, before the
	// mail invitation is sent
	StatusMailNotSent = "mail-not-sent"
)

// Member contains the information about a recipient (or the sharer) for a sharing
type Member struct {
	Status   string `json:"status"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Instance string `json:"instance,omitempty"`
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

	Owner       bool      `json:"owner"`
	Open        bool      `json:"open_sharing,omitempty"`
	Description string    `json:"description,omitempty"`
	PreviewPath string    `json:"preview_path,omitempty"`
	AppSlug     string    `json:"app_slug"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Members[0] is the owner, Members[1...] are the recipients
	Members []Member `json:"members"`

	Rules []Rule `json:"rules"`
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
	cloned.Rules = make([]Rule, len(s.Rules))
	for i := range s.Rules {
		cloned.Rules[i] = s.Rules[i]
	}
	return &cloned
}

// BeOwner is a function that setup a sharing on the cozy of its owner
func (s *Sharing) BeOwner(inst *instance.Instance, slug string) error {
	if s.AppSlug == "" {
		s.AppSlug = slug
	}
	s.CreatedAt = time.Now()
	s.UpdatedAt = s.CreatedAt
	s.Owner = true

	name, err := inst.PublicName()
	if err != nil {
		return err
	}
	email, err := inst.SettingsEMail()
	if err != nil {
		return err
	}

	s.Members = make([]Member, 1)
	s.Members[0].Status = StatusOwner
	s.Members[0].Name = name
	s.Members[0].Email = email
	s.Members[0].Instance = inst.Domain

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
		Status:   StatusMailNotSent,
		Name:     addr.Name,
		Email:    addr.Email,
		Instance: c.PrimaryCozyURL(),
	}
	s.Members = append(s.Members, m)
	return nil
}

// Create checks that the sharing is OK and it persists it in CouchDB if it is the case.
func (s *Sharing) Create(inst *instance.Instance) error {
	if len(s.Rules) == 0 {
		return ErrNoRules
	}
	if len(s.Members) < 2 {
		return ErrNoRecipients
	}

	if err := couchdb.CreateDoc(inst, s); err != nil {
		return err
	}
	// TODO create the permissions set for preview if preview_path is filled
	return nil
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

// FindMemberByShareCode returns the member that is linked to the sharing by
// the given share code
func (s *Sharing) FindMemberByShareCode(inst *instance.Instance, shareCode string) (*Member, error) {
	return nil, errors.New("Not implemented")
}

var _ couchdb.Doc = &Sharing{}
