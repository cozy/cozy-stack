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
	File *vfs.FileDoc
}

// Open will return an Opener for the given file.
func Open(inst *instance.Instance, fileID string) (*Opener, error) {
	file, err := inst.VFS().FileByID(fileID)
	if err != nil {
		return nil, err
	}
	if file.Class != "text" && file.Class != "spreadsheet" && file.Class != "slide" {
		return nil, ErrInvalidFile
	}
	return &Opener{inst: inst, File: file}, nil
}

// GetResult looks if the file can be opened locally or not, which code can be
// used in case of a shared note, and other parameters.. and returns the information.
func (o *Opener) GetResult(mode string) (jsonapi.Object, error) {
	var cfg *config.Office
	configuration := config.GetConfig().Office
	if c, ok := configuration[o.inst.ContextName]; ok {
		cfg = &c
	} else if c, ok := configuration[config.DefaultInstanceContext]; ok {
		cfg = &c
	}
	if cfg == nil || cfg.OnlyOfficeURL == "" {
		return nil, ErrNoServer
	}

	// Create a local result
	result := &apiOfficeURL{
		FileID:   o.File.ID(),
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
	download, err := o.downloadURL()
	if err != nil {
		o.inst.Logger().WithField("nspace", "office").
			Infof("Cannot build download URL: %s", err)
		return nil, ErrInternalServerError
	}
	publicName, _ := o.inst.PublicName()
	result.OO = &onlyOffice{
		URL: cfg.OnlyOfficeURL,
	}
	result.OO.Doc.Filetype = o.File.Mime
	result.OO.Doc.Key = fmt.Sprintf("%s-%s", o.File.ID(), o.File.Rev())
	result.OO.Doc.Title = o.File.DocName
	result.OO.Doc.URL = download
	result.OO.Doc.Info.Owner = publicName
	result.OO.Doc.Info.Uploaded = uploadedDate(o.File)
	result.OO.Editor.Callback = o.inst.PageURL("/office/"+o.File.ID()+"/callback", nil)
	result.OO.Editor.Lang = o.inst.Locale
	result.OO.Editor.Mode = mode

	// Enforce DocID and PublicName with local values
	result.DocID = o.File.ID()
	result.PublicName = publicName
	return result, nil
}

// downloadURL returns an URL where the Document Server can download the file.
func (o *Opener) downloadURL() (string, error) {
	path, err := o.File.Path(o.inst.VFS())
	if err != nil {
		return "", err
	}
	secret, err := vfs.GetStore().AddFile(o.inst, path)
	if err != nil {
		return "", err
	}
	return o.inst.PageURL("/files/downloads/"+secret+"/"+o.File.DocName, nil), nil
}

// uploadedDate returns the uploaded date for a file in the date format used by
// OnlyOffice
func uploadedDate(f *vfs.FileDoc) string {
	date := f.CreatedAt
	if f.CozyMetadata != nil && f.CozyMetadata.UploadedAt != nil {
		date = *f.CozyMetadata.UploadedAt
	}
	return date.Format("2006-01-02 3:04 PM")
}
