package bitwarden

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/bitwarden"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/metadata"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

type cipherRequest struct {
	Type           bitwarden.CipherType `json:"type"`
	Favorite       bool                 `json:"favorite"`
	Name           string               `json:"name"`
	Notes          string               `json:"notes"`
	FolderID       string               `json:"folderId"`
	OrganizationID string               `json:"organizationId"`
	Login          bitwarden.LoginData  `json:"login"`
	SecureNote     bitwarden.MapData    `json:"securenote"`
	Card           bitwarden.MapData    `json:"card"`
	Identity       bitwarden.MapData    `json:"identity"`
}

func (r *cipherRequest) toCipher() (*bitwarden.Cipher, error) {
	if r.OrganizationID != "" {
		return nil, errors.New("organizationId is not yet supported")
	}
	if r.Name == "" {
		return nil, errors.New("name is mandatory")
	}

	c := bitwarden.Cipher{
		Type:     r.Type,
		Name:     r.Name,
		Notes:    r.Notes,
		FolderID: r.FolderID,
	}
	switch c.Type {
	case bitwarden.LoginType:
		c.Login = &r.Login
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
	URIs []uriResponse `json:"Uris"`
}

type cipherResponse struct {
	Object         string                 `json:"Object"`
	ID             string                 `json:"Id"`
	Type           int                    `json:"Type"`
	Favorite       bool                   `json:"Favorite"`
	Name           string                 `json:"Name"`
	Notes          *string                `json:"Notes"`
	FolderID       *string                `json:"FolderId"`
	OrganizationID *string                `json:"OrganizationId"`
	Fields         *string                `json:"Fields"`
	Attachments    *string                `json:"Attachments"`
	Login          *loginResponse         `json:"Login,omitempty"`
	Username       *string                `json:"Username"`
	Password       *string                `json:"Password"`
	TOTP           *string                `json:"Totp"`
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
		res[strings.Title(k)] = v
	}
	return res
}

func newCipherResponse(c *bitwarden.Cipher) *cipherResponse {
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

	switch c.Type {
	case bitwarden.LoginType:
		if c.Login != nil {
			if c.Login.URI != "" {
				uri := uriResponse{URI: c.Login.URI, Match: nil}
				r.Login = &loginResponse{
					URIs: []uriResponse{uri},
				}
			}
			if c.Login.Username != "" {
				r.Username = &c.Login.Username
			}
			if c.Login.Password != "" {
				r.Password = &c.Login.Password
			}
			if c.Login.TOTP != "" {
				r.TOTP = &c.Login.TOTP
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

	res := newCipherResponse(cipher)
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
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	res := newCipherResponse(cipher)
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
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}

	if err := couchdb.DeleteDoc(inst, cipher); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err,
		})
	}
	return c.NoContent(http.StatusNoContent)
}
