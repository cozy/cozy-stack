package note

import (
	"github.com/cozy/cozy-stack/model/instance"
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
	doc := &apiNoteURL{
		DocID:    fileID,
		NoteID:   fileID,
		Instance: inst.ContextualDomain(),
	}
	switch config.GetConfig().Subdomains {
	case config.FlatSubdomains:
		doc.Subdomain = "flat"
	case config.NestedSubdomains:
		doc.Subdomain = "nested"
	}

	if name, err := inst.PublicName(); err == nil {
		doc.PublicName = name
	}

	// TODO check if the note is shared, and if it is the case, get info from the sharer

	return doc, nil
}
