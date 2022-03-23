package bitwarden

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/cozy/cozy-stack/model/bitwarden"
	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/metadata"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

type loginRequest struct {
	URI string `json:"uri"` // For compatibility with some clients
	*bitwarden.LoginData
}

type idsRequest struct {
	IDs []string `json:"ids"`
}

// https://github.com/bitwarden/jslib/blob/master/common/src/models/request/cipherRequest.ts
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

type importCipherRequest struct {
	Ciphers             []cipherRequest `json:"ciphers"`
	Folders             []folderRequest `json:"folders"`
	FolderRelationships []struct {
		Cipher int `json:"key"`
		Folder int `json:"value"`
	} `json:"folderRelationships"`
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

// https://github.com/bitwarden/jslib/blob/master/common/src/models/response/cipherResponse.ts
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
	DeletedDate    *time.Time             `json:"DeletedDate,omitempty"`
	Edit           bool                   `json:"Edit"`
	UseOTP         bool                   `json:"OrganizationUseTotp"`
}

func titleizeKeys(data bitwarden.MapData) map[string]interface{} {
	res := make(map[string]interface{})
	for k, v := range data {
		if k == "ssn" {
			k = "SSN"
		}
		key := []byte(k[:])
		if 'a' <= key[0] && key[0] <= 'z' {
			key[0] -= 'a' - 'A'
		}
		res[string(key)] = v
	}
	return res
}

func newCipherResponse(c *bitwarden.Cipher, setting *settings.Settings) *cipherResponse {
	r := cipherResponse{
		Object:   "cipher",
		ID:       c.CouchID,
		Type:     int(c.Type),
		Favorite: c.Favorite,
		Name:     c.Name,
		Edit:     true,
		UseOTP:   false,
	}
	if c.DeletedDate != nil {
		date := c.DeletedDate.UTC()
		r.DeletedDate = &date
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
		r.OrganizationID = &setting.OrganizationID
		r.CollectionIDs = append(r.CollectionIDs, setting.CollectionID)
	} else if c.CollectionID != "" {
		r.OrganizationID = &c.OrganizationID
		r.CollectionIDs = append(r.CollectionIDs, c.CollectionID)
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
			"error": err.Error(),
		})
	}

	setting, err := settings.Get(inst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	res := &ciphersList{Object: "list"}
	for _, f := range ciphers {
		res.Data = append(res.Data, newCipherResponse(f, setting))
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
			"error": err.Error(),
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
			"error": err.Error(),
		})
	}

	setting, err := settings.Get(inst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	_ = settings.UpdateRevisionDate(inst, setting)
	res := newCipherResponse(cipher, setting)
	return c.JSON(http.StatusOK, res)
}

// CreateSharedCipher is the handler for creating a shared cipher.
func CreateSharedCipher(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.POST, consts.BitwardenCiphers); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}

	var req struct {
		Cipher        cipherRequest `json:"cipher"`
		CollectionIDs []string      `json:"collectionIds"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid JSON",
		})
	}

	cipher, err := req.Cipher.toCipher()
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
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

	setting, err := settings.Get(inst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}
	if len(req.CollectionIDs) != 1 {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "only one collection per organization is supported",
		})
	}
	for _, id := range req.CollectionIDs {
		if id == setting.CollectionID {
			cipher.SharedWithCozy = true
		} else {
			cipher.OrganizationID = req.Cipher.OrganizationID
			cipher.CollectionID = id
		}
	}

	if err := couchdb.CreateDoc(inst, cipher); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	_ = settings.UpdateRevisionDate(inst, setting)
	res := newCipherResponse(cipher, setting)
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
			"error": err.Error(),
		})
	}

	setting, err := settings.Get(inst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	res := newCipherResponse(cipher, setting)
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
			"error": err.Error(),
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
			"error": err.Error(),
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

	// XXX On an update, the client send the OrganizationId but not the
	// collectionIds.
	if req.OrganizationID != "" {
		if old.SharedWithCozy {
			cipher.SharedWithCozy = true
		} else {
			cipher.OrganizationID = req.OrganizationID
			cipher.CollectionID = old.CollectionID
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
			"error": err.Error(),
		})
	}

	setting, err := settings.Get(inst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	_ = settings.UpdateRevisionDate(inst, setting)
	res := newCipherResponse(cipher, setting)
	return c.JSON(http.StatusOK, res)
}

// DeleteCipher is the handler for the route to delete a cipher.
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
			"error": err.Error(),
		})
	}

	if err := couchdb.DeleteDoc(inst, cipher); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	_ = settings.UpdateRevisionDate(inst, nil)
	return c.NoContent(http.StatusOK)
}

// SoftDeleteCipher is the handler for the route to soft delete a cipher.
// See https://github.com/bitwarden/server/pull/684 for the bitwarden implementation
func SoftDeleteCipher(c echo.Context) error {
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

	cipher := &bitwarden.Cipher{}
	if err := couchdb.GetDoc(inst, consts.BitwardenCiphers, id, cipher); err != nil {
		if couchdb.IsNotFoundError(err) {
			return c.JSON(http.StatusNotFound, echo.Map{
				"error": "not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	setting, err := settings.Get(inst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	cipher.Metadata.ChangeUpdatedAt()
	cipher.DeletedDate = &cipher.Metadata.UpdatedAt
	if err := couchdb.UpdateDoc(inst, cipher); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}
	_ = settings.UpdateRevisionDate(inst, setting)

	return c.NoContent(http.StatusOK)
}

// RestoreCipher is the handler for the route to restore a soft-deleted cipher.
// See https://github.com/bitwarden/server/pull/684 for the bitwarden implementation
func RestoreCipher(c echo.Context) error {
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

	cipher := &bitwarden.Cipher{}
	if err := couchdb.GetDoc(inst, consts.BitwardenCiphers, id, cipher); err != nil {
		if couchdb.IsNotFoundError(err) {
			return c.JSON(http.StatusNotFound, echo.Map{
				"error": "not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	setting, err := settings.Get(inst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	cipher.DeletedDate = nil
	cipher.Metadata.ChangeUpdatedAt()
	if err := couchdb.UpdateDoc(inst, cipher); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}
	_ = settings.UpdateRevisionDate(inst, setting)
	return c.NoContent(http.StatusOK)
}

// BulkDeleteCiphers is the handler for the route to delete ciphers in bulk.
func BulkDeleteCiphers(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.DELETE, consts.BitwardenCiphers); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}

	var req idsRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid JSON",
		})
	}
	if len(req.IDs) == 0 {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "Request missing ids field",
		})
	}

	var ciphers []bitwarden.Cipher
	keys := couchdb.AllDocsRequest{Keys: req.IDs}
	if err := couchdb.GetAllDocs(inst, consts.BitwardenCiphers, &keys, &ciphers); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}
	docs := make([]couchdb.Doc, len(ciphers))
	for i := range ciphers {
		docs[i] = ciphers[i].Clone()
	}
	if err := couchdb.BulkDeleteDocs(inst, consts.BitwardenCiphers, docs); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	_ = settings.UpdateRevisionDate(inst, nil)
	return c.NoContent(http.StatusOK)
}

// BulkSoftDeleteCiphers is the handler for the route to soft delete ciphers in bulk.
func BulkSoftDeleteCiphers(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.PUT, consts.BitwardenCiphers); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}

	var req idsRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid JSON",
		})
	}
	if len(req.IDs) == 0 {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "Request missing ids field",
		})
	}

	var ciphers []bitwarden.Cipher
	keys := couchdb.AllDocsRequest{Keys: req.IDs}
	if err := couchdb.GetAllDocs(inst, consts.BitwardenCiphers, &keys, &ciphers); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}
	olds := make([]interface{}, len(ciphers))
	docs := make([]interface{}, len(ciphers))
	for i := range ciphers {
		olds[i] = ciphers[i]
		cipher := ciphers[i].Clone().(*bitwarden.Cipher)
		cipher.Metadata.ChangeUpdatedAt()
		cipher.DeletedDate = &cipher.Metadata.UpdatedAt
		docs[i] = cipher
	}
	if err := couchdb.BulkUpdateDocs(inst, consts.BitwardenCiphers, docs, olds); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	_ = settings.UpdateRevisionDate(inst, nil)
	return c.NoContent(http.StatusOK)
}

// BulkRestoreCiphers is the handler for the route to restore ciphers in bulk.
func BulkRestoreCiphers(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.PUT, consts.BitwardenCiphers); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}

	var req idsRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid JSON",
		})
	}
	if len(req.IDs) == 0 {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "Request missing ids field",
		})
	}

	var ciphers []bitwarden.Cipher
	keys := couchdb.AllDocsRequest{Keys: req.IDs}
	if err := couchdb.GetAllDocs(inst, consts.BitwardenCiphers, &keys, &ciphers); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}
	olds := make([]interface{}, len(ciphers))
	docs := make([]interface{}, len(ciphers))
	for i := range ciphers {
		olds[i] = ciphers[i]
		cipher := ciphers[i].Clone().(*bitwarden.Cipher)
		cipher.Metadata.ChangeUpdatedAt()
		cipher.DeletedDate = nil
		docs[i] = cipher
	}
	if err := couchdb.BulkUpdateDocs(inst, consts.BitwardenCiphers, docs, olds); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	setting, err := settings.Get(inst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}
	_ = settings.UpdateRevisionDate(inst, setting)

	res := &ciphersList{Object: "list"}
	for i := range docs {
		cipher := docs[i].(*bitwarden.Cipher)
		res.Data = append(res.Data, newCipherResponse(cipher, setting))
	}
	return c.JSON(http.StatusOK, res)
}

// https://github.com/bitwarden/jslib/blob/master/common/src/models/request/cipherShareRequest.ts
type shareCipherRequest struct {
	Cipher        cipherRequest `json:"cipher"`
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
			"error": err.Error(),
		})
	}

	var req shareCipherRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		inst.Logger().WithNamespace("bitwarden").
			Infof("Bad JSON: %v", err)
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid JSON",
		})
	}
	cipher, err := req.Cipher.toCipher()
	if err != nil {
		inst.Logger().WithNamespace("bitwarden").
			Infof("Bad cipher: %v", err)
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}
	if req.Cipher.OrganizationID == "" {
		inst.Logger().WithNamespace("bitwarden").
			Infof("Bad organization: %v", req)
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "organizationId not provided",
		})
	}

	setting, err := settings.Get(inst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	if len(req.CollectionIDs) != 1 {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "only one collection per organization is supported",
		})
	}
	for _, id := range req.CollectionIDs {
		if id == setting.CollectionID {
			cipher.SharedWithCozy = true
			cipher.OrganizationID = ""
			cipher.CollectionID = ""
		} else {
			cipher.OrganizationID = req.Cipher.OrganizationID
			cipher.CollectionID = id
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
			"error": err.Error(),
		})
	}

	_ = settings.UpdateRevisionDate(inst, setting)
	res := newCipherResponse(cipher, setting)
	return c.JSON(http.StatusOK, res)
}

// ImportCiphers is used to import ciphers and folders in bulk.
func ImportCiphers(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.POST, consts.BitwardenCiphers); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}

	var req importCipherRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid JSON",
		})
	}

	// Import the folders
	folders := make([]interface{}, len(req.Folders))
	olds := make([]interface{}, len(req.Folders))
	for i, folder := range req.Folders {
		folders[i] = folder.toFolder()
	}
	if err := couchdb.BulkUpdateDocs(inst, consts.BitwardenFolders, folders, olds); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	// Import the ciphers
	ciphers := make([]interface{}, len(req.Ciphers))
	olds = make([]interface{}, len(req.Ciphers))
	for i, cipherReq := range req.Ciphers {
		cipher, err := cipherReq.toCipher()
		if err != nil {
			return c.JSON(http.StatusBadRequest, echo.Map{
				"error": err.Error(),
			})
		}
		for _, kv := range req.FolderRelationships {
			if kv.Cipher == i && kv.Folder < len(folders) {
				cipher.FolderID = folders[kv.Folder].(*bitwarden.Folder).ID()
			}
		}
		ciphers[i] = cipher
	}
	if err := couchdb.BulkUpdateDocs(inst, consts.BitwardenCiphers, ciphers, olds); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	// Update the revision date
	setting, err := settings.Get(inst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}
	_ = settings.UpdateRevisionDate(inst, setting)

	// Send in the realtime hub an event to force a sync
	go func() {
		time.Sleep(1 * time.Second)
		payload := couchdb.JSONDoc{
			M: map[string]interface{}{
				"import": true,
			},
			Type: consts.BitwardenCiphers,
		}
		realtime.GetHub().Publish(inst, realtime.EventNotify, &payload, nil)
	}()

	return c.NoContent(http.StatusOK)
}
