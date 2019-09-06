package bitwarden

import (
	"net/http"

	"github.com/cozy/cozy-stack/model/bitwarden"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

type profileResponse struct {
	ID            string        `json:"Id"`
	Name          string        `json:"Name"`
	Email         string        `json:"Email"`
	EmailVerified bool          `json:"EmailVerified"`
	Premium       bool          `json:"Premium"`
	Hint          interface{}   `json:"MasterPasswordHint"`
	Culture       string        `json:"Culture"`
	TwoFactor     bool          `json:"TwoFactorEnabled"`
	Key           string        `json:"Key"`
	PrivateKey    interface{}   `json:"PrivateKey"`
	SStamp        string        `json:"SecurityStamp"`
	Organizations []interface{} `json:"Organizations"`
	Object        string        `json:"Object"`
}

func newProfileResponse(inst *instance.Instance) (*profileResponse, error) {
	settings, err := inst.SettingsDocument()
	if err != nil {
		return nil, err
	}
	name, _ := settings.M["public_name"].(string)
	salt := inst.PassphraseSalt()
	p := &profileResponse{
		ID:            settings.ID(),
		Name:          name,
		Email:         string(salt),
		EmailVerified: false,
		Hint:          nil,
		Culture:       inst.Locale,
		TwoFactor:     false,
		Key:           inst.PassphraseKey,
		PrivateKey:    nil,
		SStamp:        inst.PassphraseStamp,
		Organizations: nil,
		Object:        "profile",
	}
	return p, nil
}

type domainsResponse struct {
	EquivalentDomains       interface{} `json:"EquivalentDomains"`
	GlobalEquivalentDomains interface{} `json:"GlobalEquivalentDomains"`
	Object                  string      `json:"Object"`
}

type syncResponse struct {
	Profile *profileResponse  `json:"Profile"`
	Folders []*folderResponse `json:"Folders"`
	Ciphers []*cipherResponse `json:"Ciphers"`
	Domains *domainsResponse  `json:"Domains"`
	Object  string            `json:"Object"`
}

func newSyncResponse(profile *profileResponse, ciphers []*bitwarden.Cipher, folders []*bitwarden.Folder) *syncResponse {
	foldersResponse := make([]*folderResponse, len(folders))
	for i, f := range folders {
		foldersResponse[i] = newFolderResponse(f)
	}
	ciphersResponse := make([]*cipherResponse, len(ciphers))
	for i, c := range ciphers {
		ciphersResponse[i] = newCipherResponse(c)
	}
	domains := &domainsResponse{
		EquivalentDomains:       nil,
		GlobalEquivalentDomains: nil,
		Object:                  "domains",
	}
	return &syncResponse{
		Profile: profile,
		Folders: foldersResponse,
		Ciphers: ciphersResponse,
		Domains: domains,
		Object:  "sync",
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

	profile, err := newProfileResponse(inst)
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

	res := newSyncResponse(profile, ciphers, folders)
	return c.JSON(http.StatusOK, res)
}
