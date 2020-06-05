package note

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/model/vfs"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/logger"
)

type apiNoteURL struct {
	DocID      string `json:"_id,omitempty"`
	NoteID     string `json:"note_id"`
	Protocol   string `json:"protocol"`
	Subdomain  string `json:"subdomain"`
	Instance   string `json:"instance"`
	Sharecode  string `json:"sharecode,omitempty"`
	PublicName string `json:"public_name,omitempty"`
}

func (n *apiNoteURL) ID() string                             { return n.DocID }
func (n *apiNoteURL) Rev() string                            { return "" }
func (n *apiNoteURL) DocType() string                        { return consts.NotesURL }
func (n *apiNoteURL) Clone() couchdb.Doc                     { cloned := *n; return &cloned }
func (n *apiNoteURL) SetID(id string)                        { n.DocID = id }
func (n *apiNoteURL) SetRev(rev string)                      {}
func (n *apiNoteURL) Relationships() jsonapi.RelationshipMap { return nil }
func (n *apiNoteURL) Included() []jsonapi.Object             { return nil }
func (n *apiNoteURL) Links() *jsonapi.LinksList              { return nil }
func (n *apiNoteURL) Fetch(field string) []string            { return nil }

// Opener can be used to find the parameters for creating the URL where the
// note can be opened.
type Opener struct {
	inst      *instance.Instance
	file      *vfs.FileDoc
	sharing   *sharing.Sharing // can be nil
	code      string
	clientID  string
	memberKey string
}

// Open will return an Opener for the given file.
func Open(inst *instance.Instance, fileID string) (*Opener, error) {
	file, err := inst.VFS().FileByID(fileID)
	if err != nil {
		return nil, err
	}

	// Check that the file is a note
	if _, err := fromMetadata(file); err != nil {
		return nil, err
	}

	// Looks if the note is shared
	sharing, err := getSharing(inst, fileID)
	if err != nil {
		return nil, err
	}

	return &Opener{inst: inst, file: file, sharing: sharing}, nil
}

func getSharing(inst *instance.Instance, fileID string) (*sharing.Sharing, error) {
	sid := consts.Files + "/" + fileID
	var ref sharing.SharedRef
	if err := couchdb.GetDoc(inst, consts.Shared, sid, &ref); err != nil {
		if couchdb.IsNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}

	for sharingID, info := range ref.Infos {
		if info.Removed {
			continue
		}
		var sharing sharing.Sharing
		if err := couchdb.GetDoc(inst, consts.Sharings, sharingID, &sharing); err != nil {
			return nil, err
		}
		if sharing.Active {
			return &sharing, nil
		}
	}
	return nil, nil
}

// AddShareByLinkCode can be used to give a sharecode that can be used to open
// the note, when the note is in a directory shared by link.
func (o *Opener) AddShareByLinkCode(code string) {
	o.code = code
}

// CheckPermission takes the permission doc, and checks that the user has the
// right to open the note.
func (o *Opener) CheckPermission(pdoc *permission.Permission, sharingID string) error {
	// If a note is opened from a preview of a sharing, and nobody has accepted
	// the sharing until now, the io.cozy.shared document for the note has not
	// been created, and we need to fill the sharing by another way.
	if o.sharing == nil && pdoc.Type == permission.TypeSharePreview {
		parts := strings.SplitN(pdoc.SourceID, "/", 2)
		if len(parts) != 2 {
			return sharing.ErrInvalidSharing
		}
		sharingID := parts[1]
		var sharing sharing.Sharing
		if err := couchdb.GetDoc(o.inst, consts.Sharings, sharingID, &sharing); err != nil {
			return err
		}
		o.sharing = &sharing
		preview, err := permission.GetForSharePreview(o.inst, sharingID)
		if err != nil {
			return err
		}
		for k, v := range preview.Codes {
			if v == o.code {
				o.memberKey = k
			}
		}
	}

	// If a note is opened via a token for cozy-to-cozy sharing, then the note
	// must be in this sharing, or the stack should refuse to open the note.
	if sharingID != "" && o.sharing != nil && o.sharing.ID() == sharingID {
		o.clientID = pdoc.SourceID
		return nil
	}

	fs := o.inst.VFS()
	return vfs.Allows(fs, pdoc.Permissions, permission.GET, o.file)
}

// GetResult looks if the note can be opened locally or not, which code can be
// used in case of a shared note, and other parameters.. and returns the information.
func (o *Opener) GetResult(memberIndex int, readOnly bool) (jsonapi.Object, error) {
	var result *apiNoteURL
	var err error
	if o.shouldOpenLocally() {
		result, err = o.openLocalNote(memberIndex, readOnly)
	} else {
		result, err = o.openSharedNote()
	}
	if err != nil {
		return nil, err
	}

	// Enforce DocID and PublicName with local values
	result.DocID = o.file.ID()
	if name, err := o.inst.PublicName(); err == nil {
		result.PublicName = name
	}
	return result, nil
}

func (o *Opener) shouldOpenLocally() bool {
	if o.sharing == nil {
		return true
	}
	u, err := url.Parse(o.file.CozyMetadata.CreatedOn)
	if err != nil {
		return true
	}
	return o.inst.HasDomain(u.Host)
}

func (o *Opener) openLocalNote(memberIndex int, readOnly bool) (*apiNoteURL, error) {
	code, err := o.getSharecode(memberIndex, readOnly)
	if err != nil {
		return nil, err
	}
	doc := &apiNoteURL{
		NoteID:    o.file.ID(),
		Instance:  o.inst.ContextualDomain(),
		Sharecode: code,
	}
	switch config.GetConfig().Subdomains {
	case config.FlatSubdomains:
		doc.Subdomain = "flat"
	case config.NestedSubdomains:
		doc.Subdomain = "nested"
	}
	doc.Protocol = "https"
	if build.IsDevRelease() {
		doc.Protocol = "http"
	}
	return doc, nil
}

func (o *Opener) openSharedNote() (*apiNoteURL, error) {
	s := o.sharing
	var creds *sharing.Credentials
	var creator *sharing.Member
	var memberIndex int
	readOnly := s.ReadOnlyRules()

	if s.Owner {
		domain := o.file.CozyMetadata.CreatedOn
		for i, m := range s.Members {
			if i == 0 {
				continue // Skip the owner
			}
			if m.Instance == domain || m.Instance+"/" == domain {
				creds = &s.Credentials[i-1]
				creator = &s.Members[i]
			}
		}
		if o.clientID != "" && !readOnly {
			for i, c := range s.Credentials {
				if c.InboundClientID == o.clientID {
					memberIndex = i + 1
					readOnly = s.Members[i+1].ReadOnly
				}
			}
		}
	} else {
		creds = &s.Credentials[0]
		creator = &s.Members[0]
	}

	logger.WithNamespace("foobar").Warnf("creator.Status = %v", creator.Status)
	if creator == nil ||
		(creator.Status != sharing.MemberStatusReady && creator.Status != sharing.MemberStatusOwner) {
		// If the creator of the note is no longer in the sharing, the owner of
		// the sharing takes the lead, and if the sharing is revoked, any
		// member can edit the note on their instance.
		if o.clientID == "" {
			o.sharing = nil
		}
		return o.openLocalNote(memberIndex, readOnly)
	}

	xoredID := sharing.XorID(o.file.ID(), creds.XorKey)
	u, err := url.Parse(creator.Instance)
	if err != nil {
		return nil, err
	}
	opts := &request.Options{
		Method: http.MethodGet,
		Scheme: u.Scheme,
		Domain: u.Host,
		Path:   "/notes/" + xoredID + "/open",
		Queries: url.Values{
			"SharingID":   {s.ID()},
			"MemberIndex": {strconv.FormatInt(int64(memberIndex), 10)},
			"ReadOnly":    {strconv.FormatBool(readOnly)},
		},
		Headers: request.Headers{
			"Accept":        "application/vnd.api+json",
			"Authorization": "Bearer " + creds.AccessToken.AccessToken,
		},
	}
	res, err := request.Req(opts)
	if res != nil && res.StatusCode/100 == 4 {
		res, err = sharing.RefreshToken(o.inst, s, creator, creds, opts, nil)
	}
	if err != nil {
		return nil, sharing.ErrInternalServerError
	}
	defer res.Body.Close()
	var doc apiNoteURL
	if _, err := jsonapi.Bind(res.Body, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func (o *Opener) getSharecode(memberIndex int, readOnly bool) (string, error) {
	s := o.sharing
	if s == nil || (o.clientID == "" && o.memberKey == "") {
		return o.code, nil
	}

	var member *sharing.Member
	var err error
	if o.memberKey != "" {
		// Preview of a cozy-to-cozy sharing
		for i, m := range s.Members {
			if m.Instance == o.memberKey || m.Email == o.memberKey {
				member = &s.Members[i]
			}
		}
		if member == nil {
			return "", sharing.ErrMemberNotFound
		}
		if member.ReadOnly {
			readOnly = true
		} else {
			readOnly = s.ReadOnlyRules()
		}
	} else if s.Owner {
		member, err = s.FindMemberByInboundClientID(o.clientID)
		if err != nil {
			return "", err
		}
		if member.ReadOnly {
			readOnly = true
		} else {
			readOnly = s.ReadOnlyRules()
		}
	} else {
		// Trust the owner
		if memberIndex < 0 && memberIndex >= len(s.Members) {
			return "", sharing.ErrMemberNotFound
		}
		member = &s.Members[memberIndex]
	}

	if readOnly {
		return o.getPreviewCode(member)
	}
	return o.getInteractCode(member)
}

// getPreviewCode returns a sharecode that can be used for reading the note. It
// uses a share-preview token.
func (o *Opener) getPreviewCode(member *sharing.Member) (string, error) {
	var codes map[string]string
	preview, err := permission.GetForSharePreview(o.inst, o.sharing.ID())
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			codes, err = o.sharing.CreatePreviewPermissions(o.inst)
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

	return "", ErrInvalidFile
}

// getInteractCode returns a sharecode that can be use for reading and writing
// the note. It uses a share-interact token.
func (o *Opener) getInteractCode(member *sharing.Member) (string, error) {
	interact, err := permission.GetForShareInteract(o.inst, o.sharing.ID())
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			return o.sharing.CreateInteractPermissions(o.inst, member)
		}
		return "", err
	}

	// If we already have a code for this member, let's use it
	for key, code := range interact.Codes {
		if key == member.Instance || key == member.Email {
			return code, nil
		}
	}

	// Else, create a code and add it to the permission doc
	key := member.Email
	if key == "" {
		key = member.Instance
	}
	code, err := o.inst.CreateShareCode(key)
	if err != nil {
		return "", err
	}
	interact.Codes[key] = code
	if err := couchdb.UpdateDoc(o.inst, interact); err != nil {
		return "", err
	}
	return code, nil
}
