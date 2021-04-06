package office

import (
	"github.com/cozy/cozy-stack/client/request"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	jwt "gopkg.in/dgrijalva/jwt-go.v3"
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
	URL   string `json:"url,omitempty"`
	Token string `json:"token,omitempty"`
	Type  string `json:"documentType"`
	Doc   struct {
		Filetype string `json:"filetype,omitempty"`
		Key      string `json:"key"`
		Title    string `json:"title,omitempty"`
		URL      string `json:"url"`
		Info     struct {
			Owner    string `json:"owner,omitempty"`
			Uploaded string `json:"uploaded,omitempty"`
		} `json:"info"`
	} `json:"document"`
	Editor struct {
		Callback string `json:"callbackUrl"`
		Lang     string `json:"lang,omitempty"`
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

func (o *apiOfficeURL) sign(cfg *config.Office) (string, error) {
	if cfg == nil || cfg.InboxSecret == "" {
		return "", nil
	}

	claims := *o.OO
	claims.URL = ""
	claims.Doc.Filetype = ""
	claims.Doc.Title = ""
	claims.Doc.Info.Owner = ""
	claims.Doc.Info.Uploaded = ""
	claims.Editor.Lang = ""
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &claims)
	return token.SignedString([]byte(cfg.InboxSecret))
}

// Valid is a method of the jwt.Claims interface
func (o *onlyOffice) Valid() error { return nil }

// Opener can be used to find the parameters for opening an office document.
type Opener struct {
	*sharing.FileOpener
}

// Open will return an Opener for the given file.
func Open(inst *instance.Instance, fileID string) (*Opener, error) {
	file, err := inst.VFS().FileByID(fileID)
	if err != nil {
		return nil, err
	}

	// Check that the file is an office document
	if !isOfficeDocument(file) {
		return nil, ErrInvalidFile
	}

	opener, err := sharing.NewFileOpener(inst, file)
	if err != nil {
		return nil, err
	}
	return &Opener{opener}, nil
}

// GetResult looks if the file can be opened locally or not, which code can be
// used in case of a shared note, and other parameters.. and returns the information.
func (o *Opener) GetResult(memberIndex int, readOnly bool) (jsonapi.Object, error) {
	var result *apiOfficeURL
	var err error
	if o.ShouldOpenLocally() {
		result, err = o.openLocalDocument(memberIndex, readOnly)
	} else {
		result, err = o.openSharedDocument()
	}
	if err != nil {
		return nil, err
	}

	result.DocID = o.File.ID()
	return result, nil
}

func (o *Opener) openLocalDocument(memberIndex int, readOnly bool) (*apiOfficeURL, error) {
	cfg := getConfig(o.Inst.ContextName)
	if cfg == nil || cfg.OnlyOfficeURL == "" {
		return nil, ErrNoServer
	}

	// Create a local result
	code, err := o.GetSharecode(memberIndex, readOnly)
	if err != nil {
		return nil, err
	}
	params := o.OpenLocalFile(code)
	doc := apiOfficeURL{
		DocID:     params.FileID,
		Protocol:  params.Protocol,
		Subdomain: params.Subdomain,
		Instance:  params.Instance,
		Sharecode: params.Sharecode,
	}

	// Fill the parameters for the Document Server
	mode := "edit"
	if readOnly {
		mode = "view"
	}
	download, err := o.downloadURL()
	if err != nil {
		o.Inst.Logger().WithField("nspace", "office").
			Infof("Cannot build download URL: %s", err)
		return nil, ErrInternalServerError
	}
	key, err := GetStore().AddDoc(o.Inst, o.File.ID(), o.File.Rev())
	if err != nil {
		o.Inst.Logger().WithField("nspace", "office").
			Infof("Cannot add doc to store: %s", err)
		return nil, ErrInternalServerError
	}
	publicName, _ := o.Inst.PublicName()
	doc.PublicName = publicName
	doc.OO = &onlyOffice{
		URL:  cfg.OnlyOfficeURL,
		Type: documentType(o.File),
	}
	doc.OO.Doc.Filetype = o.File.Mime
	doc.OO.Doc.Key = key
	doc.OO.Doc.Title = o.File.DocName
	doc.OO.Doc.URL = download
	doc.OO.Doc.Info.Owner = publicName
	doc.OO.Doc.Info.Uploaded = uploadedDate(o.File)
	doc.OO.Editor.Callback = o.Inst.PageURL("/office/callback", nil)
	doc.OO.Editor.Lang = o.Inst.Locale
	doc.OO.Editor.Mode = mode

	token, err := doc.sign(cfg)
	if err != nil {
		return nil, err
	}
	doc.OO.Token = token
	return &doc, nil
}

func (o *Opener) openSharedDocument() (*apiOfficeURL, error) {
	prepared, err := o.PrepareRequestForSharedFile()
	if err != nil {
		return nil, err
	}
	if prepared.Opts == nil {
		return o.openLocalDocument(prepared.MemberIndex, prepared.ReadOnly)
	}

	prepared.Opts.Path = "/office/" + prepared.XoredID + "/open"
	res, err := request.Req(prepared.Opts)
	if res != nil && res.StatusCode/100 == 4 {
		res, err = sharing.RefreshToken(o.Inst, err, o.Sharing, prepared.Creator,
			prepared.Creds, prepared.Opts, nil)
	}
	if err != nil {
		return nil, sharing.ErrInternalServerError
	}
	defer res.Body.Close()
	var doc apiOfficeURL
	if _, err := jsonapi.Bind(res.Body, &doc); err != nil {
		return nil, err
	}
	publicName, _ := o.Inst.PublicName()
	doc.PublicName = publicName
	return &doc, nil
}

// downloadURL returns an URL where the Document Server can download the file.
func (o *Opener) downloadURL() (string, error) {
	path, err := o.File.Path(o.Inst.VFS())
	if err != nil {
		return "", err
	}
	secret, err := vfs.GetStore().AddFile(o.Inst, path)
	if err != nil {
		return "", err
	}
	return o.Inst.PageURL("/files/downloads/"+secret+"/"+o.File.DocName, nil), nil
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

// documentType returns the document type parameter for Only Office
// Cf https://api.onlyoffice.com/editors/config/#documentType
func documentType(f *vfs.FileDoc) string {
	switch f.Class {
	case "spreadsheet":
		return "cell"
	case "slide":
		return "slide"
	default:
		return "word"
	}
}

func isOfficeDocument(f *vfs.FileDoc) bool {
	switch f.Class {
	case "spreadsheet", "slide", "text":
		return true
	default:
		return false
	}
}

func getConfig(contextName string) *config.Office {
	configuration := config.GetConfig().Office
	if c, ok := configuration[contextName]; ok {
		return &c
	} else if c, ok := configuration[config.DefaultInstanceContext]; ok {
		return &c
	}
	return nil
}
