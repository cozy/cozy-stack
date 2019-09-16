package bitwarden

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/bitwarden"
	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// https://github.com/bitwarden/jslib/blob/master/src/models/response/profileResponse.ts
type profileResponse struct {
	ID            string                  `json:"Id"`
	Name          string                  `json:"Name"`
	Email         string                  `json:"Email"`
	EmailVerified bool                    `json:"EmailVerified"`
	Premium       bool                    `json:"Premium"`
	Hint          interface{}             `json:"MasterPasswordHint"`
	Culture       string                  `json:"Culture"`
	TwoFactor     bool                    `json:"TwoFactorEnabled"`
	Key           string                  `json:"Key"`
	PrivateKey    interface{}             `json:"PrivateKey"`
	SStamp        string                  `json:"SecurityStamp"`
	Organizations []*organizationResponse `json:"Organizations"`
	Object        string                  `json:"Object"`
}

func newProfileResponse(inst *instance.Instance, settings *settings.Settings) (*profileResponse, error) {
	doc, err := inst.SettingsDocument()
	if err != nil {
		return nil, err
	}
	name, _ := doc.M["public_name"].(string)
	salt := inst.PassphraseSalt()
	var organizations []*organizationResponse
	if orga, err := getCozyOrganizationResponse(inst, settings); err == nil {
		organizations = append(organizations, orga)
	}
	p := &profileResponse{
		ID:            inst.ID(),
		Name:          name,
		Email:         string(salt),
		EmailVerified: false,
		Premium:       true,
		Hint:          nil,
		Culture:       inst.Locale,
		TwoFactor:     false,
		Key:           settings.Key,
		SStamp:        settings.SecurityStamp,
		Organizations: organizations,
		Object:        "profile",
	}
	if settings.PrivateKey != "" {
		p.PrivateKey = settings.PrivateKey
	}
	if settings.PassphraseHint != "" {
		p.Hint = settings.PassphraseHint
	}
	return p, nil
}

// https://github.com/bitwarden/jslib/blob/master/src/models/response/domainsResponse.ts
type domainsResponse struct {
	EquivalentDomains       []interface{} `json:"EquivalentDomains"`
	GlobalEquivalentDomains []interface{} `json:"GlobalEquivalentDomains"`
	Object                  string        `json:"Object"`
}

// https://github.com/bitwarden/jslib/blob/master/src/models/response/syncResponse.ts
type syncResponse struct {
	Profile     *profileResponse      `json:"Profile"`
	Folders     []*folderResponse     `json:"Folders"`
	Ciphers     []*cipherResponse     `json:"Ciphers"`
	Collections []*collectionResponse `json:"Collections"`
	Domains     *domainsResponse      `json:"Domains"`
	Object      string                `json:"Object"`
}

func newSyncResponse(settings *settings.Settings,
	profile *profileResponse,
	ciphers []*bitwarden.Cipher,
	folders []*bitwarden.Folder,
) *syncResponse {
	foldersResponse := make([]*folderResponse, len(folders))
	for i, f := range folders {
		foldersResponse[i] = newFolderResponse(f)
	}
	ciphersResponse := make([]*cipherResponse, len(ciphers))
	for i, c := range ciphers {
		ciphersResponse[i] = newCipherResponse(c, settings)
	}
	domains := &domainsResponse{
		EquivalentDomains:       []interface{}{},
		GlobalEquivalentDomains: []interface{}{},
		Object:                  "domains",
	}
	var collections []*collectionResponse
	if coll, err := getCozyCollectionResponse(settings); err == nil {
		collections = append(collections, coll)
	}
	return &syncResponse{
		Profile:     profile,
		Folders:     foldersResponse,
		Ciphers:     ciphersResponse,
		Collections: collections,
		Domains:     domains,
		Object:      "sync",
	}
}

// Sync is the handler for the main endpoint of the bitwarden API. It is used
// by the client as a one-way sync: it fetches all objects from the server to
// update its local database.
func Sync(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.GET, consts.BitwardenCiphers); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}
	settings, err := settings.Get(inst)
	if err != nil {
		return err
	}

	profile, err := newProfileResponse(inst, settings)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	var ciphers []*bitwarden.Cipher
	req := &couchdb.AllDocsRequest{}
	if err := couchdb.GetAllDocs(inst, consts.BitwardenCiphers, req, &ciphers); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	var folders []*bitwarden.Folder
	req = &couchdb.AllDocsRequest{}
	if err := couchdb.GetAllDocs(inst, consts.BitwardenFolders, req, &folders); err != nil {
		if couchdb.IsNoDatabaseError(err) {
			_ = couchdb.CreateDB(inst, consts.BitwardenFolders)
		} else {
			return c.JSON(http.StatusInternalServerError, echo.Map{
				"error": err,
			})
		}
	}

	res := newSyncResponse(settings, profile, ciphers, folders)
	return c.JSON(http.StatusOK, res)
}
