package sharing

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/permissions"
)

const (
	// StateLen is the number of bytes for the OAuth state parameter
	StateLen = 16
)

// Sharing contains all the information about a sharing.
type Sharing struct {
	SID  string `json:"_id,omitempty"`
	SRev string `json:"_rev,omitempty"`

	// Triggers keep record of which triggers are active
	Triggers struct {
		Track     bool `json:"track,omitempty"`
		Replicate bool `json:"replicate,omitempty"`
		Upload    bool `json:"upload,omitempty"`
	} `json:"triggers"`

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

// BeOwner initializes a sharing on the cozy of its owner
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
	if err := s.ValidateRules(); err != nil {
		return nil, err
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
	if err := s.ValidateRules(); err != nil {
		return err
	}
	if len(s.Members) < 2 {
		return ErrNoRecipients
	}
	// TODO check members

	s.Active = false
	s.Owner = false
	s.UpdatedAt = time.Now()
	s.Credentials = make([]Credentials, 1)

	return couchdb.CreateNamedDocWithDB(inst, s)
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

var _ couchdb.Doc = &Sharing{}
