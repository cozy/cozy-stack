package sharing

import (
	"path/filepath"

	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/settings"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
)

// apiExcalidrawURL holds the parameters to build the URL where an excalidraw
// document can be opened. It uses the io.cozy.files doctype, as no dedicated
// doctype is needed.
type apiExcalidrawURL struct {
	DocID      string `json:"_id,omitempty"`
	Protocol   string `json:"protocol"`
	Subdomain  string `json:"subdomain"`
	Instance   string `json:"instance"`
	Sharecode  string `json:"sharecode,omitempty"`
	PublicName string `json:"public_name,omitempty"`
}

func (e *apiExcalidrawURL) ID() string                             { return e.DocID }
func (e *apiExcalidrawURL) Rev() string                            { return "" }
func (e *apiExcalidrawURL) DocType() string                        { return consts.Files }
func (e *apiExcalidrawURL) Clone() couchdb.Doc                     { cloned := *e; return &cloned }
func (e *apiExcalidrawURL) SetID(id string)                        { e.DocID = id }
func (e *apiExcalidrawURL) SetRev(rev string)                      {}
func (e *apiExcalidrawURL) Relationships() jsonapi.RelationshipMap { return nil }
func (e *apiExcalidrawURL) Included() []jsonapi.Object             { return nil }
func (e *apiExcalidrawURL) Links() *jsonapi.LinksList              { return nil }
func (e *apiExcalidrawURL) Fetch(field string) []string            { return nil }

// ExcalidrawOpener can be used to find the parameters for creating the URL
// where an excalidraw document can be opened.
type ExcalidrawOpener struct {
	*FileOpener
}

// OpenExcalidraw returns an ExcalidrawOpener for the given file.
func OpenExcalidraw(inst *instance.Instance, fileID string) (*ExcalidrawOpener, error) {
	file, err := inst.VFS().FileByID(fileID)
	if err != nil {
		return nil, err
	}

	// Check that the file is an excalidraw document
	if filepath.Ext(file.DocName) != consts.ExcalidrawExtension {
		return nil, ErrCannotOpenFile
	}

	opener, err := NewFileOpener(inst, file)
	if err != nil {
		return nil, err
	}
	return &ExcalidrawOpener{opener}, nil
}

// GetResult looks if the excalidraw document can be opened locally or not,
// which code can be used in case of a shared document, and other parameters,
// and returns the information.
func (o *ExcalidrawOpener) GetResult(memberIndex int, readOnly bool) (jsonapi.Object, error) {
	var result *apiExcalidrawURL
	var err error
	if o.ShouldOpenLocally() {
		result, err = o.openLocalDocument(memberIndex, readOnly)
	} else {
		result, err = o.openSharedDocument()
	}
	if err != nil {
		return nil, err
	}

	// Enforce DocID and PublicName with local values
	result.DocID = o.File.ID()
	if name, err := settings.PublicName(o.Inst); err == nil {
		result.PublicName = name
	}
	return result, nil
}

func (o *ExcalidrawOpener) openLocalDocument(memberIndex int, readOnly bool) (*apiExcalidrawURL, error) {
	code, err := o.GetSharecode(memberIndex, readOnly)
	if err != nil {
		return nil, err
	}
	params := o.OpenLocalFile(code)
	doc := apiExcalidrawURL{
		Protocol:  params.Protocol,
		Subdomain: params.Subdomain,
		Instance:  params.Instance,
		Sharecode: params.Sharecode,
	}
	return &doc, nil
}

func (o *ExcalidrawOpener) openSharedDocument() (*apiExcalidrawURL, error) {
	prepared, err := o.PrepareRequestForSharedFile()
	if err != nil {
		return nil, err
	}
	if prepared.Opts == nil {
		return o.openLocalDocument(prepared.MemberIndex, prepared.ReadOnly)
	}

	prepared.Opts.Path = "/excalidraw/" + prepared.XoredID + "/open"
	res, err := request.Req(prepared.Opts)
	if res != nil && res.StatusCode/100 == 4 {
		res, err = RefreshToken(o.Inst, res, err, o.Sharing, prepared.Creator,
			prepared.Creds, prepared.Opts, nil)
	}
	if err != nil {
		return nil, ErrInternalServerError
	}
	defer res.Body.Close()
	var doc apiExcalidrawURL
	if _, err := jsonapi.Bind(res.Body, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}
