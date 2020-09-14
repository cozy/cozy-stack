package sharing

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/contact"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/couchdb/mango"
	"github.com/cozy/cozy-stack/pkg/metadata"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/realtime"
	multierror "github.com/hashicorp/go-multierror"
)

const (
	// StateLen is the number of bytes for the OAuth state parameter
	StateLen = 16
)

// Triggers keep record of which triggers are active
type Triggers struct {
	TrackID     string `json:"track_id,omitempty"`
	ReplicateID string `json:"replicate_id,omitempty"`
	UploadID    string `json:"upload_id,omitempty"`
}

// Sharing contains all the information about a sharing.
type Sharing struct {
	SID  string `json:"_id,omitempty"`
	SRev string `json:"_rev,omitempty"`

	Triggers    Triggers  `json:"triggers"`
	Active      bool      `json:"active,omitempty"`
	Owner       bool      `json:"owner,omitempty"`
	Open        bool      `json:"open_sharing,omitempty"`
	Description string    `json:"description,omitempty"`
	AppSlug     string    `json:"app_slug"`
	PreviewPath string    `json:"preview_path,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	NbFiles     int       `json:"initial_number_of_files_to_sync,omitempty"`
	ShortcutID  string    `json:"shortcut_id,omitempty"`

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
	for i := range cloned.Rules {
		cloned.Rules[i].Values = make([]string, len(s.Rules[i].Values))
		copy(cloned.Rules[i].Values, s.Rules[i].Values)
	}
	cloned.Members = make([]Member, len(s.Members))
	copy(cloned.Members, s.Members)
	cloned.Credentials = make([]Credentials, len(s.Credentials))
	copy(cloned.Credentials, s.Credentials)
	for i := range s.Credentials {
		if s.Credentials[i].Client != nil {
			cloned.Credentials[i].Client = s.Credentials[i].Client.Clone()
		}
		if s.Credentials[i].AccessToken != nil {
			cloned.Credentials[i].AccessToken = s.Credentials[i].AccessToken.Clone()
		}
		cloned.Credentials[i].XorKey = make([]byte, len(s.Credentials[i].XorKey))
		copy(cloned.Credentials[i].XorKey, s.Credentials[i].XorKey)
	}
	return &cloned
}

// ReadOnlyFlag returns true only if the given instance is declared a read-only
// member of the sharing.
func (s *Sharing) ReadOnlyFlag() bool {
	if !s.Owner {
		for i, m := range s.Members {
			if i == 0 {
				continue // skip owner
			}
			if m.Instance != "" {
				return m.ReadOnly
			}
		}
	}
	return false
}

// ReadOnlyRules returns true if the rules forbid that a change on the
// recipient's cozy instance can be propagated to the sharer's cozy.
func (s *Sharing) ReadOnlyRules() bool {
	for _, rule := range s.Rules {
		if rule.HasSync() {
			return false
		}
	}
	return true
}

// ReadOnly returns true if the member has the read-only flag, or if the rules
// forces a read-only mode.
func (s *Sharing) ReadOnly() bool {
	return s.ReadOnlyFlag() || s.ReadOnlyRules()
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
	s.Members[0].PublicName = name
	s.Members[0].Email = email
	s.Members[0].Instance = inst.PageURL("", nil)

	return nil
}

// CreatePreviewPermissions creates the permissions doc for previewing this sharing,
// or updates it with the new codes if the document already exists
func (s *Sharing) CreatePreviewPermissions(inst *instance.Instance) (map[string]string, error) {
	doc, _ := permission.GetForSharePreview(inst, s.SID)

	codes := make(map[string]string, len(s.Members)-1)

	for i, m := range s.Members {
		if i == 0 {
			continue
		}
		var err error
		var previousVal string
		var okShare bool
		key := m.Email
		if key == "" {
			key = m.Instance
		}

		// Checks that we don't already have a sharing code
		if doc != nil {
			previousVal, okShare = doc.Codes[key]
		}

		if !okShare {
			codes[key], err = inst.CreateShareCode(key)
			if err != nil {
				return nil, err
			}
		} else {
			codes[key] = previousVal
		}
	}

	set := make(permission.Set, len(s.Rules))
	getVerb := permission.VerbSplit("GET")
	for i, rule := range s.Rules {
		set[i] = permission.Rule{
			Type:     rule.DocType,
			Title:    rule.Title,
			Verbs:    getVerb,
			Selector: rule.Selector,
			Values:   rule.Values,
		}
	}

	if doc != nil {
		if doc.Metadata != nil {
			err := doc.Metadata.UpdatedByApp(s.AppSlug, "")
			if err != nil {
				return nil, err
			}
		}
		doc.Codes = codes
		if err := couchdb.UpdateDoc(inst, doc); err != nil {
			return nil, err
		}
	} else {
		md := metadata.New()
		md.CreatedByApp = s.AppSlug
		subdoc := permission.Permission{
			Permissions: set,
			Metadata:    md,
		}
		_, err := permission.CreateSharePreviewSet(inst, s.SID, codes, subdoc)
		if err != nil {
			return nil, err
		}
	}
	return codes, nil
}

// CreateInteractPermissions creates the permissions doc for reading and
// writing a note inside this sharing.
func (s *Sharing) CreateInteractPermissions(inst *instance.Instance, m *Member) (string, error) {
	key := m.Email
	if key == "" {
		key = m.Instance
	}
	code, err := inst.CreateShareCode(key)
	if err != nil {
		return "", err
	}
	codes := map[string]string{key: code}

	set := make(permission.Set, len(s.Rules))
	getVerb := permission.ALL
	for i, rule := range s.Rules {
		set[i] = permission.Rule{
			Type:     rule.DocType,
			Title:    rule.Title,
			Verbs:    getVerb,
			Selector: rule.Selector,
			Values:   rule.Values,
		}
	}

	md := metadata.New()
	md.CreatedByApp = s.AppSlug
	doc := permission.Permission{
		Permissions: set,
		Metadata:    md,
	}

	_, err = permission.CreateShareInteractSet(inst, s.SID, codes, doc)
	if err != nil {
		return "", err
	}
	return code, nil
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

	for i, m := range s.Members {
		if m.Email != "" {
			if c, err := contact.FindByEmail(inst, m.Email); err == nil {
				s.Members[i].Name = c.PrimaryName()
			}
		}
	}

	err := couchdb.CreateNamedDocWithDB(inst, s)
	if couchdb.IsConflictError(err) {
		old, errb := FindSharing(inst, s.SID)
		if errb != nil {
			return errb
		}
		if old.Owner {
			return ErrInvalidSharing
		}
		if old.Active {
			return ErrAlreadyAccepted
		}
		s.ShortcutID = old.ShortcutID
		s.SRev = old.SRev
		err = couchdb.UpdateDoc(inst, s)
	}
	return err
}

// Revoke remove the credentials for all members, contact them, removes the
// triggers and set the active flag to false.
func (s *Sharing) Revoke(inst *instance.Instance) error {
	var errm error

	if !s.Owner {
		return ErrInvalidSharing
	}
	for i := range s.Credentials {
		if err := s.RevokeMember(inst, i+1); err != nil {
			errm = multierror.Append(errm, err)
		}
		if err := s.ClearLastSequenceNumbers(inst, &s.Members[i+1]); err != nil {
			return err
		}
	}
	if err := s.RemoveTriggers(inst); err != nil {
		return err
	}
	if err := RemoveSharedRefs(inst, s.SID); err != nil {
		return err
	}
	if s.PreviewPath != "" {
		if err := s.RevokePreviewPermissions(inst); err != nil {
			return err
		}
	}
	s.Active = false
	if err := couchdb.UpdateDoc(inst, s); err != nil {
		return err
	}
	return errm
}

// RevokePreviewPermissions ensure that the permissions for the preview page
// are no longer valid.
func (s *Sharing) RevokePreviewPermissions(inst *instance.Instance) error {
	perms, err := permission.GetForSharePreview(inst, s.SID)
	if err != nil {
		return err
	}
	now := time.Now()
	perms.ExpiresAt = &now
	return couchdb.UpdateDoc(inst, perms)
}

// RevokeRecipient revoke only one recipient on the sharer. After that, if the
// sharing has still at least one active member, we keep it as is. Else, we
// desactive the sharing.
func (s *Sharing) RevokeRecipient(inst *instance.Instance, index int) error {
	if !s.Owner {
		return ErrInvalidSharing
	}
	if err := s.RevokeMember(inst, index); err != nil {
		return err
	}
	if err := s.ClearLastSequenceNumbers(inst, &s.Members[index]); err != nil {
		return err
	}
	return s.NoMoreRecipient(inst)
}

// RevokeRecipientBySelf revoke the sharing on the recipient side
func (s *Sharing) RevokeRecipientBySelf(inst *instance.Instance, sharingDirTrashed bool) error {
	if s.Owner || len(s.Members) == 0 {
		return ErrInvalidSharing
	}
	if err := s.RevokeOwner(inst); err != nil {
		return err
	}
	if err := s.RemoveTriggers(inst); err != nil {
		return err
	}
	if err := s.ClearLastSequenceNumbers(inst, &s.Members[0]); err != nil {
		return err
	}
	if err := RemoveSharedRefs(inst, s.SID); err != nil {
		return err
	}
	if !sharingDirTrashed && s.FirstFilesRule() != nil {
		if err := s.RemoveSharingDir(inst); err != nil {
			return err
		}
	}
	s.Active = false

	for i, m := range s.Members {
		if i > 0 && m.Instance != "" {
			s.Members[i].Status = MemberStatusRevoked
			break
		}
	}

	return couchdb.UpdateDoc(inst, s)
}

// RemoveTriggers remove all the triggers associated to this sharing
func (s *Sharing) RemoveTriggers(inst *instance.Instance) error {
	if err := removeSharingTrigger(inst, s.Triggers.TrackID); err != nil {
		return err
	}
	if err := removeSharingTrigger(inst, s.Triggers.ReplicateID); err != nil {
		return err
	}
	if err := removeSharingTrigger(inst, s.Triggers.UploadID); err != nil {
		return err
	}
	s.Triggers = Triggers{}
	return nil
}

func removeSharingTrigger(inst *instance.Instance, triggerID string) error {
	if triggerID != "" {
		sched := job.System()
		if err := sched.DeleteTrigger(inst, triggerID); err != nil {
			return err
		}
	}
	return nil
}

// RevokeByNotification is called on the recipient side, after a revocation
// performed by the sharer
func (s *Sharing) RevokeByNotification(inst *instance.Instance) error {
	if s.Owner {
		return ErrInvalidSharing
	}
	if err := DeleteOAuthClient(inst, &s.Members[0], &s.Credentials[0]); err != nil {
		return err
	}
	if err := s.RemoveTriggers(inst); err != nil {
		return err
	}
	if err := s.ClearLastSequenceNumbers(inst, &s.Members[0]); err != nil {
		return err
	}
	if err := RemoveSharedRefs(inst, s.SID); err != nil {
		return err
	}
	if s.FirstFilesRule() != nil {
		if err := s.RemoveSharingDir(inst); err != nil {
			return err
		}
	}
	s.Credentials = nil
	s.Active = false

	for i, m := range s.Members {
		if i > 0 && m.Instance != "" {
			s.Members[i].Status = MemberStatusRevoked
			break
		}
	}

	return couchdb.UpdateDoc(inst, s)
}

// RevokeRecipientByNotification is called on the sharer side, after a
// revocation performed by the recipient
func (s *Sharing) RevokeRecipientByNotification(inst *instance.Instance, m *Member) error {
	if !s.Owner {
		return ErrInvalidSharing
	}
	c := s.FindCredentials(m)
	if err := DeleteOAuthClient(inst, m, c); err != nil {
		return err
	}
	if err := s.ClearLastSequenceNumbers(inst, m); err != nil {
		return err
	}
	m.Status = MemberStatusRevoked
	*c = Credentials{}

	return s.NoMoreRecipient(inst)
}

// NoMoreRecipient cleans up the sharing if there is no more active recipient
func (s *Sharing) NoMoreRecipient(inst *instance.Instance) error {
	for _, m := range s.Members {
		if m.Status == MemberStatusReady {
			return couchdb.UpdateDoc(inst, s)
		}
	}
	if err := s.RemoveTriggers(inst); err != nil {
		return err
	}
	if err := RemoveSharedRefs(inst, s.SID); err != nil {
		return err
	}
	s.Active = false
	return couchdb.UpdateDoc(inst, s)
}

// FindSharing retrieves a sharing document from its ID
func FindSharing(db prefixer.Prefixer, sharingID string) (*Sharing, error) {
	res := &Sharing{}
	err := couchdb.GetDoc(db, consts.Sharings, sharingID, res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// FindSharings retrieves an array of sharing documents from their IDs
func FindSharings(db prefixer.Prefixer, sharingIDs []string) ([]*Sharing, error) {
	req := &couchdb.AllDocsRequest{
		Keys: sharingIDs,
	}
	var res []*Sharing
	err := couchdb.GetAllDocs(db, consts.Sharings, req, &res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// GetSharingsByDocType returns all the sharings for the given doctype
func GetSharingsByDocType(inst *instance.Instance, docType string) (map[string]*Sharing, error) {
	req := &couchdb.ViewRequest{
		Key:         docType,
		IncludeDocs: true,
	}
	var res couchdb.ViewResponse
	err := couchdb.ExecView(inst, couchdb.SharingsByDocTypeView, req, &res)
	if err != nil {
		return nil, err
	}
	sharings := make(map[string]*Sharing, len(res.Rows))

	for _, row := range res.Rows {
		var doc Sharing
		err := json.Unmarshal(row.Doc, &doc)
		if err != nil {
			return nil, err
		}
		// Avoid duplicates, i.e. a set a rules having the same doctype
		sID := row.Value.(string)
		if _, ok := sharings[sID]; !ok {
			sharings[sID] = &doc
		}
	}
	return sharings, nil
}

func findIntentForRedirect(inst *instance.Instance, webapp *app.WebappManifest, doctype string) (*app.Intent, string) {
	action := "SHARING"
	if webapp != nil {
		if intent := webapp.FindIntent(action, doctype); intent != nil {
			return intent, webapp.Slug()
		}
	}
	var mans []app.WebappManifest
	err := couchdb.GetAllDocs(inst, consts.Apps, &couchdb.AllDocsRequest{}, &mans)
	if err != nil {
		return nil, ""
	}
	for _, man := range mans {
		if intent := man.FindIntent(action, doctype); intent != nil {
			return intent, man.Slug()
		}
	}
	return nil, ""
}

// RedirectAfterAuthorizeURL returns the URL for the redirection after a user
// has authorized a sharing.
func (s *Sharing) RedirectAfterAuthorizeURL(inst *instance.Instance) *url.URL {
	doctype := s.Rules[0].DocType
	webapp, _ := app.GetWebappBySlug(inst, s.AppSlug)

	if intent, slug := findIntentForRedirect(inst, webapp, doctype); intent != nil {
		u := inst.SubDomain(slug)
		parts := strings.SplitN(intent.Href, "#", 2)
		if len(parts[0]) > 0 {
			u.Path = parts[0]
		}
		if len(parts) == 2 && len(parts[1]) > 0 {
			u.Fragment = parts[1]
		}
		u.RawQuery = "sharing=" + s.SID
		return u
	}

	if webapp == nil {
		return inst.DefaultRedirection()
	}
	u := inst.SubDomain(webapp.Slug())
	u.RawQuery = "sharing=" + s.SID
	return u
}

// EndInitial is used to finish the initial sync phase of a sharing
func (s *Sharing) EndInitial(inst *instance.Instance) error {
	if s.NbFiles == 0 {
		return nil
	}
	s.NbFiles = 0
	if err := couchdb.UpdateDoc(inst, s); err != nil {
		return err
	}
	doc := couchdb.JSONDoc{
		Type: consts.SharingsInitialSync,
		M:    map[string]interface{}{"_id": s.SID},
	}
	realtime.GetHub().Publish(inst, realtime.EventDelete, &doc, nil)
	return nil
}

// GetSharecode returns a sharecode for the given client that can be used to
// preview the sharing.
func GetSharecode(inst *instance.Instance, sharingID, clientID string) (string, error) {
	var s Sharing
	if err := couchdb.GetDoc(inst, consts.Sharings, sharingID, &s); err != nil {
		return "", err
	}
	member, err := s.FindMemberByInboundClientID(clientID)
	if err != nil {
		return "", err
	}
	var codes map[string]string
	preview, err := permission.GetForSharePreview(inst, sharingID)
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			codes, err = s.CreatePreviewPermissions(inst)
		}
		if err != nil {
			return "", err
		}
	} else {
		codes = preview.Codes
	}
	for key, code := range codes {
		if key == member.Instance || key == member.Email {
			return code, nil
		}
	}
	return "", ErrMemberNotFound
}

var _ couchdb.Doc = &Sharing{}

// GetSharecodeFromShortcut returns the sharecode from the shortcut for this sharing.
func (s *Sharing) GetSharecodeFromShortcut(inst *instance.Instance) (string, error) {
	key := []string{consts.Sharings, s.SID}
	end := []string{key[0], key[1], couchdb.MaxString}
	req := &couchdb.ViewRequest{
		StartKey:    key,
		EndKey:      end,
		IncludeDocs: true,
	}
	var res couchdb.ViewResponse
	err := couchdb.ExecView(inst, couchdb.FilesReferencedByView, req, &res)
	if err != nil {
		return "", ErrInternalServerError
	}
	if len(res.Rows) == 0 {
		return "", ErrInvalidSharing
	}

	fs := inst.VFS()
	file, err := fs.FileByID(res.Rows[0].ID)
	if err != nil || file.Mime != consts.ShortcutMimeType {
		return "", ErrInvalidSharing
	}
	f, err := fs.OpenFile(file)
	if err != nil {
		return "", ErrInternalServerError
	}
	defer f.Close()
	var buf bytes.Buffer
	_, err = buf.ReadFrom(f)
	if err != nil {
		return "", ErrInternalServerError
	}
	u, err := url.Parse(buf.String())
	if err != nil {
		return "", ErrInternalServerError
	}
	code := u.Query().Get("sharecode")
	if code == "" {
		return "", ErrInvalidSharing
	}
	return code, nil
}

func (s *Sharing) cleanShortcutID(inst *instance.Instance) string {
	if s.ShortcutID == "" {
		return ""
	}

	var parentID string
	fs := inst.VFS()
	if file, err := fs.FileByID(s.ShortcutID); err == nil {
		parentID = file.DirID
		if err := fs.DestroyFile(file); err != nil {
			return ""
		}
	}
	s.ShortcutID = ""
	_ = couchdb.UpdateDoc(inst, s)
	return parentID
}

// GetPreviewURL asks the owner's Cozy the URL for previewing the sharing.
func (s *Sharing) GetPreviewURL(inst *instance.Instance, state string) (string, error) {
	u, err := url.Parse(s.Members[0].Instance)
	if s.Members[0].Instance == "" || err != nil {
		return "", ErrInvalidSharing
	}
	body, err := json.Marshal(map[string]interface{}{"state": state})
	if err != nil {
		return "", err
	}
	res, err := request.Req(&request.Options{
		Method: http.MethodPost,
		Scheme: u.Scheme,
		Domain: u.Host,
		Path:   "/sharings/" + s.SID + "/preview-url",
		Headers: request.Headers{
			"Accept":       "application/json",
			"Content-Type": "application/json",
		},
		Body: bytes.NewReader(body),
	})
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	var data map[string]interface{}
	if err = json.NewDecoder(res.Body).Decode(&data); err != nil {
		return "", ErrRequestFailed
	}

	previewURL, ok := data["url"].(string)
	if !ok || previewURL == "" {
		return "", ErrRequestFailed
	}
	return previewURL, nil
}

// AddShortcut creates a shortcut for this sharing on the local instance.
func (s *Sharing) AddShortcut(inst *instance.Instance, state string) error {
	previewURL, err := s.GetPreviewURL(inst, state)
	if err != nil {
		return err
	}
	return s.CreateShortcut(inst, previewURL, true)
}

// CountNewShortcuts returns the number of shortcuts to a sharing that have not
// been seen.
func CountNewShortcuts(inst *instance.Instance) (int, error) {
	count := 0
	perPage := 1000
	list := make([]couchdb.JSONDoc, 0, perPage)
	var bookmark string
	for {
		req := &couchdb.FindRequest{
			UseIndex: "by-sharing-status",
			Selector: mango.Equal("metadata.sharing.status", "new"),
			Limit:    perPage,
			Bookmark: bookmark,
		}
		res, err := couchdb.FindDocsRaw(inst, consts.Files, req, &list)
		if err != nil {
			return 0, err
		}
		count += len(list)
		if len(list) < perPage {
			return count, nil
		}
		bookmark = res.Bookmark
	}
}
