package bitwarden

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/bitwarden"
	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/metadata"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

type loginRequest struct {
	URI string `json:"uri"` // For compatibility with some clients
	*bitwarden.LoginData
}

// https://github.com/bitwarden/jslib/blob/master/src/models/request/cipherRequest.ts
type cipherRequest struct {
	Type           bitwarden.CipherType `json:"type"`
	Favorite       bool                 `json:"favorite"`
	Name           string               `json:"name"`
	Notes          string               `json:"notes"`
	FolderID       string               `json:"folderId"`
	OrganizationID string               `json:"organizationId"`
	Login          loginRequest         `json:"login"`
	Fields         []bitwarden.Field    `json:"fields"`
	SecureNote     bitwarden.MapData    `json:"securenote"`
	Card           bitwarden.MapData    `json:"card"`
	Identity       bitwarden.MapData    `json:"identity"`
}

func (r *cipherRequest) toCipher() (*bitwarden.Cipher, error) {
	if r.Name == "" {
		return nil, errors.New("name is mandatory")
	}

	c := bitwarden.Cipher{
		Type:     r.Type,
		Favorite: r.Favorite,
		Name:     r.Name,
		Notes:    r.Notes,
		FolderID: r.FolderID,
		Fields:   r.Fields,
	}
	switch c.Type {
	case bitwarden.LoginType:
		if r.Login.LoginData == nil {
			r.Login.LoginData = &bitwarden.LoginData{}
		}
		if r.Login.URI != "" {
			u := bitwarden.LoginURI{URI: r.Login.URI, Match: nil}
			r.Login.URIs = append(r.Login.URIs, u)
		}
		c.Login = r.Login.LoginData
	case bitwarden.SecureNoteType:
		c.Data = &r.SecureNote
	case bitwarden.CardType:
		c.Data = &r.Card
	case bitwarden.IdentityType:
		c.Data = &r.Identity
	default:
		return nil, errors.New("type has an unknown value")
	}

	md := metadata.New()
	md.DocTypeVersion = bitwarden.DocTypeVersion
	c.Metadata = md
	return &c, nil
}

type uriResponse struct {
	URI   string      `json:"Uri"`
	Match interface{} `json:"Match"`
}

type loginResponse struct {
	URIs     []uriResponse `json:"Uris"`
	Username *string       `json:"Username"`
	Password *string       `json:"Password"`
	RevDate  *string       `json:"PasswordRevisionDate"`
	TOTP     *string       `json:"Totp"`
}

type fieldResponse struct {
	Type  int    `json:"Type"`
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

// https://github.com/bitwarden/jslib/blob/master/src/models/response/cipherResponse.ts
type cipherResponse struct {
	Object         string                 `json:"Object"`
	ID             string                 `json:"Id"`
	Type           int                    `json:"Type"`
	Favorite       bool                   `json:"Favorite"`
	Name           string                 `json:"Name"`
	Notes          *string                `json:"Notes"`
	FolderID       *string                `json:"FolderId"`
	OrganizationID *string                `json:"OrganizationId"`
	CollectionIDs  []string               `json:"CollectionIds"`
	Fields         interface{}            `json:"Fields"`
	Attachments    *string                `json:"Attachments"`
	Login          *loginResponse         `json:"Login,omitempty"`
	SecureNote     map[string]interface{} `json:"SecureNote,omitempty"`
	Card           map[string]interface{} `json:"Card,omitempty"`
	Identity       map[string]interface{} `json:"Identity,omitempty"`
	Date           time.Time              `json:"RevisionDate"`
	Edit           bool                   `json:"Edit"`
	UseOTP         bool                   `json:"OrganizationUseTotp"`
}

func titleizeKeys(data bitwarden.MapData) map[string]interface{} {
	res := make(map[string]interface{})
	for k, v := range data {
		key := strings.Title(k)
		if k == "ssn" {
			key = "SSN"
		}
		res[key] = v
	}
	return res
}

func newCipherResponse(c *bitwarden.Cipher, settings *settings.Settings) *cipherResponse {
	r := cipherResponse{
		Object:   "cipher",
		ID:       c.CouchID,
		Type:     int(c.Type),
		Favorite: c.Favorite,
		Name:     c.Name,
		Edit:     true,
		UseOTP:   false,
	}
	if c.Notes != "" {
		r.Notes = &c.Notes
	}
	if c.FolderID != "" {
		r.FolderID = &c.FolderID
	}
	if c.Metadata != nil {
		r.Date = c.Metadata.UpdatedAt.UTC()
	}
	if c.SharedWithCozy {
		r.OrganizationID = &settings.OrganizationID
		r.CollectionIDs = append(r.CollectionIDs, settings.CollectionID)
	}

	if len(c.Fields) > 0 {
		fields := make([]fieldResponse, len(c.Fields))
		for i, f := range c.Fields {
			fields[i] = fieldResponse{
				Type:  f.Type,
				Name:  f.Name,
				Value: f.Value,
			}
		}
		r.Fields = fields
	}

	switch c.Type {
	case bitwarden.LoginType:
		if c.Login != nil {
			r.Login = &loginResponse{}
			if len(c.Login.URIs) > 0 {
				r.Login.URIs = make([]uriResponse, len(c.Login.URIs))
				for i, u := range c.Login.URIs {
					r.Login.URIs[i] = uriResponse{URI: u.URI, Match: u.Match}
				}
			}
			if c.Login.Username != "" {
				r.Login.Username = &c.Login.Username
			}
			if c.Login.Password != "" {
				r.Login.Password = &c.Login.Password
			}
			if c.Login.RevDate != "" {
				r.Login.RevDate = &c.Login.RevDate
			}
			if c.Login.TOTP != "" {
				r.Login.TOTP = &c.Login.TOTP
			}
		}
	case bitwarden.SecureNoteType:
		if c.Data != nil {
			r.SecureNote = titleizeKeys(*c.Data)
		}
	case bitwarden.CardType:
		if c.Data != nil {
			r.Card = titleizeKeys(*c.Data)
		}
	case bitwarden.IdentityType:
		if c.Data != nil {
			r.Identity = titleizeKeys(*c.Data)
		}
	}

	return &r
}

type ciphersList struct {
	Data   []*cipherResponse `json:"Data"`
	Object string            `json:"Object"`
}

// ListCiphers is the route for listing the Bitwarden ciphers.
// No pagination yet.
func ListCiphers(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.GET, consts.BitwardenCiphers); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}

	var ciphers []*bitwarden.Cipher
	req := &couchdb.AllDocsRequest{}
	if err := couchdb.GetAllDocs(inst, consts.BitwardenCiphers, req, &ciphers); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	settings, err := settings.Get(inst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	res := &ciphersList{Object: "list"}
	for _, f := range ciphers {
		res.Data = append(res.Data, newCipherResponse(f, settings))
	}
	return c.JSON(http.StatusOK, res)
}

// CreateCipher is the handler for creating a cipher: login, secure note, etc.
func CreateCipher(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.POST, consts.BitwardenCiphers); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}

	var req cipherRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid JSON",
		})
	}

	cipher, err := req.toCipher()
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err,
		})
	}

	if cipher.FolderID != "" {
		folder := &bitwarden.Folder{}
		if err := couchdb.GetDoc(inst, consts.BitwardenFolders, cipher.FolderID, folder); err != nil {
			return c.JSON(http.StatusBadRequest, echo.Map{
				"error": "folder not found",
			})
		}
	}

	if err := couchdb.CreateDoc(inst, cipher); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	settings, err := settings.Get(inst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	res := newCipherResponse(cipher, settings)
	return c.JSON(http.StatusOK, res)
}

// GetCipher returns information about a single cipher.
func GetCipher(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.GET, consts.BitwardenCiphers); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}

	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusNotFound, echo.Map{
			"error": "missing id",
		})
	}

	cipher := &bitwarden.Cipher{}
	if err := couchdb.GetDoc(inst, consts.BitwardenCiphers, id, cipher); err != nil {
		if couchdb.IsNotFoundError(err) {
			return c.JSON(http.StatusNotFound, echo.Map{
				"error": "not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	settings, err := settings.Get(inst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	res := newCipherResponse(cipher, settings)
	return c.JSON(http.StatusOK, res)
}

// UpdateCipher is the route for changing a cipher.
func UpdateCipher(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.PUT, consts.BitwardenCiphers); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}

	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusNotFound, echo.Map{
			"error": "missing id",
		})
	}

	old := &bitwarden.Cipher{}
	if err := couchdb.GetDoc(inst, consts.BitwardenCiphers, id, old); err != nil {
		if couchdb.IsNotFoundError(err) {
			return c.JSON(http.StatusNotFound, echo.Map{
				"error": "not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	var req cipherRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid JSON",
		})
	}
	cipher, err := req.toCipher()
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err,
		})
	}

	if cipher.FolderID != "" && cipher.FolderID != old.FolderID {
		folder := &bitwarden.Folder{}
		if err := couchdb.GetDoc(inst, consts.BitwardenFolders, cipher.FolderID, folder); err != nil {
			return c.JSON(http.StatusBadRequest, echo.Map{
				"error": "folder not found",
			})
		}
	}

	if old.Metadata != nil {
		cipher.Metadata = old.Metadata.Clone()
	}
	cipher.Metadata.ChangeUpdatedAt()
	cipher.SetID(old.ID())
	cipher.SetRev(old.Rev())
	if err := couchdb.UpdateDocWithOld(inst, cipher, old); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	settings, err := settings.Get(inst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	res := newCipherResponse(cipher, settings)
	return c.JSON(http.StatusOK, res)
}

// DeleteCipher is the handler for the route to delete a folder.
func DeleteCipher(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.DELETE, consts.BitwardenCiphers); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}

	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusNotFound, echo.Map{
			"error": "missing id",
		})
	}

	cipher := &bitwarden.Cipher{}
	if err := couchdb.GetDoc(inst, consts.BitwardenCiphers, id, cipher); err != nil {
		if couchdb.IsNotFoundError(err) {
			return c.JSON(http.StatusNotFound, echo.Map{
				"error": "not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	if err := couchdb.DeleteDoc(inst, cipher); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}
	return c.NoContent(http.StatusOK)
}

type shareCipherRequest struct {
	Cipher        cipherRequest `json:"Cipher"`
	CollectionIDs []string      `json:"collectionIds"`
}

// ShareCipher is used to share a cipher with an organization.
func ShareCipher(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.PUT, consts.BitwardenCiphers); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}

	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusNotFound, echo.Map{
			"error": "missing id",
		})
	}

	old := &bitwarden.Cipher{}
	if err := couchdb.GetDoc(inst, consts.BitwardenCiphers, id, old); err != nil {
		if couchdb.IsNotFoundError(err) {
			return c.JSON(http.StatusNotFound, echo.Map{
				"error": "not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	var req shareCipherRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		inst.Logger().WithField("nspace", "bitwarden").
			Infof("Bad JSON: %v", err)
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid JSON",
		})
	}
	cipher, err := req.Cipher.toCipher()
	if err != nil {
		inst.Logger().WithField("nspace", "bitwarden").
			Infof("Bad cipher: %v", err)
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err,
		})
	}
	if req.Cipher.OrganizationID == "" {
		inst.Logger().WithField("nspace", "bitwarden").
			Infof("Bad organization: %v", req)
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "organizationId not provided",
		})
	}
	settings, err := settings.Get(inst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}
	if len(req.CollectionIDs) != 1 {
		inst.Logger().WithField("nspace", "bitwarden").
			Infof("Bad collection: %v", req.CollectionIDs)
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "generic collectionIds is not supported",
		})
	}
	for _, id := range req.CollectionIDs {
		if id != settings.CollectionID {
			inst.Logger().WithField("nspace", "bitwarden").
				Infof("Bad collection: %s vs %s", id, settings.CollectionID)
			return c.JSON(http.StatusBadRequest, echo.Map{
				"error": "generic collectionIds is not supported",
			})
		}
		cipher.SharedWithCozy = true
	}

	if old.Metadata != nil {
		cipher.Metadata = old.Metadata.Clone()
	}
	cipher.Metadata.ChangeUpdatedAt()
	cipher.SetID(old.ID())
	cipher.SetRev(old.Rev())
	if err := couchdb.UpdateDocWithOld(inst, cipher, old); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	res := newCipherResponse(cipher, settings)
	return c.JSON(http.StatusOK, res)
}
