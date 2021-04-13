package note

import (
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
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
	*sharing.FileOpener
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

	opener, err := sharing.NewFileOpener(inst, file)
	if err != nil {
		return nil, err
	}
	return &Opener{opener}, nil
}

// GetResult looks if the note can be opened locally or not, which code can be
// used in case of a shared note, and other parameters.. and returns the information.
func (o *Opener) GetResult(memberIndex int, readOnly bool) (jsonapi.Object, error) {
	var result *apiNoteURL
	var err error
	if o.ShouldOpenLocally() {
		result, err = o.openLocalNote(memberIndex, readOnly)
	} else {
		result, err = o.openSharedNote()
	}
	if err != nil {
		return nil, err
	}

	// Enforce DocID and PublicName with local values
	result.DocID = o.File.ID()
	if name, err := o.Inst.PublicName(); err == nil {
		result.PublicName = name
	}
	return result, nil
}

func (o *Opener) openLocalNote(memberIndex int, readOnly bool) (*apiNoteURL, error) {
	// If the note came from another cozy via a sharing that is now revoked, we
	// may need to recreate the trigger.
	_ = setupTrigger(o.Inst, o.File.ID())

	code, err := o.GetSharecode(memberIndex, readOnly)
	if err != nil {
		return nil, err
	}
	params := o.OpenLocalFile(code)
	doc := apiNoteURL{
		NoteID:    params.FileID,
		Protocol:  params.Protocol,
		Subdomain: params.Subdomain,
		Instance:  params.Instance,
		Sharecode: params.Sharecode,
	}
	return &doc, nil
}

func (o *Opener) openSharedNote() (*apiNoteURL, error) {
	prepared, err := o.PrepareRequestForSharedFile()
	if err != nil {
		return nil, err
	}
	if prepared.Opts == nil {
		return o.openLocalNote(prepared.MemberIndex, prepared.ReadOnly)
	}

	prepared.Opts.Path = "/notes/" + prepared.XoredID + "/open"
	res, err := request.Req(prepared.Opts)
	if res != nil && res.StatusCode/100 == 4 {
		res, err = sharing.RefreshToken(o.Inst, err, o.Sharing, prepared.Creator,
			prepared.Creds, prepared.Opts, nil)
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
