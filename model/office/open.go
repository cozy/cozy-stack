package office

import (
	"fmt"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/vfs"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
)

type apiOfficeURL struct {
	FileID     string      `json:"_id,omitempty"`
	DocID      string      `json:"document_id"`
	Subdomain  string      `json:"subdomain"`
	Protocol   string      `json:"protocol"`
	Instance   string      `json:"instance"`
	Sharecode  string      `json:"sharecode,omitempty"`
	PublicName string      `json:"public_name,omitempty"`
	OO         *onlyOffice `json:"onlyoffice,omitempty"`
}

type onlyOffice struct {
	URL string `json:"url"`
	Doc struct {
		Filetype string `json:"filetype"`
		Key      string `json:"key"`
		Title    string `json:"title"`
		URL      string `json:"url"`
		Info     struct {
			Owner    string `json:"owner,omitempty"`
			Uploaded string `json:"uploaded"`
		} `json:"info"`
	} `json:"document"`
	Editor struct {
		Callback string `json:"callbackUrl"`
		Lang     string `json:"lang"`
		Mode     string `json:"mode"`
	} `json:"editor"`
}

func (o *apiOfficeURL) ID() string                             { return o.FileID }
func (o *apiOfficeURL) Rev() string                            { return "" }
func (o *apiOfficeURL) DocType() string                        { return consts.OfficeURL }
func (o *apiOfficeURL) Clone() couchdb.Doc                     { cloned := *o; return &cloned }
func (o *apiOfficeURL) SetID(id string)                        { o.FileID = id }
func (o *apiOfficeURL) SetRev(rev string)                      {}
func (o *apiOfficeURL) Relationships() jsonapi.RelationshipMap { return nil }
func (o *apiOfficeURL) Included() []jsonapi.Object             { return nil }
func (o *apiOfficeURL) Links() *jsonapi.LinksList              { return nil }
func (o *apiOfficeURL) Fetch(field string) []string            { return nil }

// Opener can be used to find the parameters for opening an office document.
type Opener struct {
	inst *instance.Instance
	file *vfs.FileDoc
}

// Open will return an Opener for the given file.
func Open(inst *instance.Instance, fileID string) (*Opener, error) {
	file, err := inst.VFS().FileByID(fileID)
	if err != nil {
		return nil, err
	}
	// TODO check that the file is an office document
	return &Opener{inst: inst, file: file}, nil
}

// GetResult looks if the file can be opened locally or not, which code can be
// used in case of a shared note, and other parameters.. and returns the information.
func (o *Opener) GetResult() jsonapi.Object {
	// Create a local result
	result := &apiOfficeURL{
		FileID:   o.file.ID(),
		Protocol: "https",
		Instance: o.inst.ContextualDomain(),
	}
	if build.IsDevRelease() {
		result.Protocol = "http"
	}
	switch config.GetConfig().Subdomains {
	case config.FlatSubdomains:
		result.Subdomain = "flat"
	case config.NestedSubdomains:
		result.Subdomain = "nested"
	}

	// Fill the parameters for the Document Server
	result.OO = &onlyOffice{
		URL: "", // TODO read from config
	}
	result.OO.Doc.Filetype = o.file.Mime
	result.OO.Doc.Key = fmt.Sprintf("%s-%s", o.file.ID(), o.file.Rev())
	result.OO.Doc.Title = o.file.DocName
	result.OO.Doc.URL = ""           // TODO
	result.OO.Doc.Info.Owner = ""    // TODO
	result.OO.Doc.Info.Uploaded = "" // TODO
	result.OO.Editor.Callback = o.inst.PageURL("/office/"+o.file.ID()+"/callback", nil)
	result.OO.Editor.Lang = o.inst.Locale
	result.OO.Editor.Mode = "edit"

	// Enforce DocID and PublicName with local values
	result.DocID = o.file.ID()
	if name, err := o.inst.PublicName(); err == nil {
		result.PublicName = name
	}
	return result
}
