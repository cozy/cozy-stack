package note

import (
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
)

type apiNoteURL struct {
	DocID      string `json:"_id,omitempty"`
	NoteID     string `json:"note_id"`
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

// Open returns the parameters to create the URL where the note can be opened.
func Open(inst *instance.Instance, fileID string) (*apiNoteURL, error) {
	sharing, err := getSharing(inst, fileID)
	if err != nil {
		return nil, err
	}

	var doc *apiNoteURL
	if sharing == nil || sharing.Owner {
		doc = openLocalNote(inst, fileID)
	} else {
		doc, err = openSharedNote(inst, sharing, fileID)
		if err != nil {
			return nil, err
		}
	}

	doc.DocID = fileID
	if name, err := inst.PublicName(); err == nil {
		doc.PublicName = name
	}
	return doc, nil
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

func openSharedNote(inst *instance.Instance, s *sharing.Sharing, fileID string) (*apiNoteURL, error) {
	xoredID := sharing.XorID(fileID, s.Credentials[0].XorKey)
	u, err := url.Parse(s.Members[0].Instance)
	if err != nil {
		return nil, err
	}
	c := &s.Credentials[0]
	opts := &request.Options{
		Method:  http.MethodGet,
		Scheme:  u.Scheme,
		Domain:  u.Host,
		Path:    "/notes/" + xoredID,
		Queries: url.Values{"SharingID": {s.ID()}},
		Headers: request.Headers{
			"Accept":        "application/vnd.api+json",
			"Authorization": "Bearer " + c.AccessToken.AccessToken,
		},
	}
	res, err := request.Req(opts)
	if res != nil && res.StatusCode/100 == 4 {
		res, err = sharing.RefreshToken(inst, s, &s.Members[0], c, opts, nil)
	}
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, sharing.ErrInternalServerError
	}
	var doc apiNoteURL
	if _, err := jsonapi.Bind(res.Body, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func openLocalNote(inst *instance.Instance, fileID string) *apiNoteURL {
	doc := &apiNoteURL{
		NoteID:   fileID,
		Instance: inst.ContextualDomain(),
	}
	switch config.GetConfig().Subdomains {
	case config.FlatSubdomains:
		doc.Subdomain = "flat"
	case config.NestedSubdomains:
		doc.Subdomain = "nested"
	}
	return doc
}

// AddSharecodeToNoteURL adds a sharecode to allow recipients to open a shared
// note.
func AddSharecodeToNoteURL(inst *instance.Instance, n *apiNoteURL, sharingID, clientID string) {
	var s sharing.Sharing
	if err := couchdb.GetDoc(inst, consts.Sharings, sharingID, &s); err != nil {
		return
	}
	member, err := s.FindMemberByInboundClientID(clientID)
	if err != nil {
		return
	}
	preview, err := permission.GetForSharePreview(inst, sharingID)
	if err != nil {
		return
	}
	for key, code := range preview.Codes {
		if key == member.Instance || key == member.Email {
			n.Sharecode = code
		}
	}
}
