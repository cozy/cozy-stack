package sharing

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/permissions"
	multierror "github.com/hashicorp/go-multierror"
)

const (
	// StateLen is the number of bytes for the OAuth state parameter
	StateLen = 16
)

// WorkerSharingMsg is an interface for the sharing worker messages
type WorkerSharingMsg interface {
	ID() string
}

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
	cloned.Rules = make([]Rule, len(s.Rules))
	copy(cloned.Rules, s.Rules)
	cloned.Members = make([]Member, len(s.Members))
	copy(cloned.Members, s.Members)
	cloned.Credentials = make([]Credentials, len(s.Credentials))
	copy(cloned.Credentials, s.Credentials)
	return &cloned
}

// ReadOnly returns true only if the rules forbid that a change on the
// recipients' cozy instances can be propagated to the sharer's cozy.
func (s *Sharing) ReadOnly() bool {
	for _, rule := range s.Rules {
		if rule.HasSync() {
			return false
		}
	}
	return true
}

// WithPropagation returns true if no rule allows that a change can be propagated, in
// one way or another
func (s *Sharing) WithPropagation() bool {
	for _, rule := range s.Rules {
		if rule.HasSync() || rule.HasPush() {
			return true
		}
	}
	return false
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

	s.Active = false
	s.Owner = false
	s.UpdatedAt = time.Now()
	s.Credentials = make([]Credentials, 1)

	return couchdb.CreateNamedDocWithDB(inst, s)
}

// Revoke remove the credentials for all members, contact them, removes the
// triggers and set the active flag to false.
func (s *Sharing) Revoke(inst *instance.Instance) error {
	var errm error

	if !s.Owner {
		return ErrInvalidSharing
	}
	for i := range s.Credentials {
		if err := s.RevokeMember(inst, &s.Members[i+1], &s.Credentials[i]); err != nil {
			multierror.Append(errm, err)
		}
	}

	if s.WithPropagation() {
		if err := s.RemoveTriggers(inst); err != nil {
			return err
		}
	}
	if err := RemoveSharedRefs(inst, s.SID); err != nil {
		return err
	}
	s.Active = false
	if err := couchdb.UpdateDoc(inst, s); err != nil {
		return err
	}
	return errm
}

// RemoveTriggers remove all the triggers associated to this sharing
func (s *Sharing) RemoveTriggers(inst *instance.Instance) error {
	var err error
	sched := jobs.System()
	// TODO: a view on WorkerType would be more efficient
	triggers, err := sched.GetAllTriggers(inst.Domain)
	if err != nil {
		return err
	}
	for _, trigger := range triggers {
		infos := trigger.Infos()
		var sid string
		if infos.WorkerType == "share-track" {
			var m TrackMessage
			sid, err = sharingIDFromMsg(infos.Message, &m)
		} else if infos.WorkerType == "share-replicate" {
			var m ReplicateMsg
			sid, err = sharingIDFromMsg(infos.Message, &m)
		} else if infos.WorkerType == "share-upload" {
			var m UploadMsg
			sid, err = sharingIDFromMsg(infos.Message, &m)
		}
		if err != nil {
			return err
		}
		if sid == s.SID {
			if errt := sched.DeleteTrigger(inst.Domain, trigger.ID()); errt != nil {
				return errt
			}
		}
	}
	return nil
}

func sharingIDFromMsg(msg jobs.Message, wsm WorkerSharingMsg) (string, error) {
	if err := msg.Unmarshal(wsm); err != nil {
		return "", err
	}
	return wsm.ID(), nil
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

// FindSharings retrieves an array of sharing documents from their IDs
func FindSharings(db couchdb.Database, sharingIDs []string) ([]*Sharing, error) {
	var req = &couchdb.AllDocsRequest{
		Keys: sharingIDs,
	}
	var res []*Sharing
	err := couchdb.GetAllDocs(db, consts.Sharings, req, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

var _ couchdb.Doc = &Sharing{}
