// Package bitwarden exposes an API compatible with the Bitwarden Open-Soure apps.
package bitwarden

import (
	"encoding/json"
	"net/http"

	"github.com/cozy/cozy-stack/model/bitwarden"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// Prelogin tells to the client how many KDF iterations it must apply when
// hashing the master password.
func Prelogin(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	return c.JSON(http.StatusOK, echo.Map{
		"Kdf":           inst.PassphraseKdf,
		"KdfIterations": inst.PassphraseKdfIterations,
	})
}

// ChangeSecurityStamp is used by the client to change the security stamp,
// which will deconnect all the clients.
func ChangeSecurityStamp(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	var data struct {
		Hashed string `json:"masterPasswordHash"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&data); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "Missing masterPasswordHash",
		})
	}

	if err := lifecycle.CheckPassphrase(inst, []byte(data.Hashed)); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "Invalid masterPasswordHash",
		})
	}

	inst.PassphraseStamp = lifecycle.NewSecurityStamp()
	if err := couchdb.UpdateDoc(couchdb.GlobalDB, inst); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// GetToken is used by the clients to get an access token. There are two
// supported grant types: password and refresh_token. Password is used the
// first time to register the client, and gets the initial credentials, by
// sending a hash of the user password. Refresh token is used later to get
// a new access token by sending the refresh token.
func GetToken(c echo.Context) error {
	switch c.FormValue("grant_type") {
	case "password":
		return getInitialCredentials(c)
	case "refresh_token":
		return refreshToken(c)
	case "":
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "the grant_type parameter is mandatory",
		})
	default:
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid grant type",
		})
	}
}

// AccessTokenReponse is the stuct used for serializing to JSON the response
// for an access token.
type AccessTokenReponse struct {
	Type      string `json:"token_type"`
	ExpiresIn int    `json:"expires_in"`
	Access    string `json:"access_token"`
	Refresh   string `json:"refresh_token"`
	Key       string `json:"Key"`
}

func getInitialCredentials(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	pass := []byte(c.FormValue("password"))

	// Authentication
	if err := lifecycle.CheckPassphrase(inst, pass); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid password",
		})
	}
	// TODO manage 2FA

	// Register the client
	kind, softwareID := bitwarden.ParseBitwardenDeviceType(c.FormValue("deviceType"))
	client := &oauth.Client{
		RedirectURIs: []string{"https://cozy.io/"},
		ClientName:   "Bitwarden " + c.FormValue("deviceName"),
		ClientKind:   kind,
		SoftwareID:   softwareID,
	}
	if err := client.Create(inst); err != nil {
		return c.JSON(err.Code, err)
	}
	client.CouchID = client.ClientID
	// TODO send an email?

	// Create the credentials
	access, err := bitwarden.CreateAccessJWT(inst, client)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": "Can't generate access token",
		})
	}
	refresh, err := bitwarden.CreateRefreshJWT(inst, client)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": "Can't generate refresh token",
		})
	}
	key := inst.PassphraseKey

	// Send the response
	out := AccessTokenReponse{
		Type:      "Bearer",
		ExpiresIn: int(consts.AccessTokenValidityDuration.Seconds()),
		Access:    access,
		Refresh:   refresh,
		Key:       key,
	}
	return c.JSON(http.StatusOK, out)
}

func refreshToken(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	refresh := c.FormValue("refresh_token")

	// Check the refresh token
	claims, ok := oauth.ValidTokenWithSStamp(inst, consts.RefreshTokenAudience, refresh)
	if !ok || claims.Scope != bitwarden.BitwardenScope {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid refresh token",
		})
	}

	// Find the OAuth client
	client, err := oauth.FindClient(inst, claims.Subject)
	if err != nil {
		if couchErr, isCouchErr := couchdb.IsCouchError(err); isCouchErr && couchErr.StatusCode >= 500 {
			return err
		}
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "the client must be registered",
		})
	}

	// Create the credentials
	access, err := bitwarden.CreateAccessJWT(inst, client)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": "Can't generate access token",
		})
	}
	key := inst.PassphraseKey

	// Send the response
	out := AccessTokenReponse{
		Type:      "Bearer",
		ExpiresIn: int(consts.AccessTokenValidityDuration.Seconds()),
		Access:    access,
		Refresh:   refresh,
		Key:       key,
	}
	return c.JSON(http.StatusOK, out)
}

// Routes sets the routing for the Bitwarden-like API
func Routes(router *echo.Group) {
	identity := router.Group("/identity")
	identity.POST("/connect/token", GetToken)

	api := router.Group("/api")
	accounts := api.Group("/accounts")
	accounts.POST("/prelogin", Prelogin)
	accounts.POST("/security-stamp", ChangeSecurityStamp)

	ciphers := api.Group("/ciphers")
	ciphers.POST("", CreateCipher)
	ciphers.GET("/:id", GetCipher)
	ciphers.DELETE("/:id", DeleteCipher)

	folders := api.Group("/folders")
	folders.GET("", ListFolders)
	folders.POST("", CreateFolder)
	folders.GET("/:id", GetFolder)
	folders.POST("/:id", RenameFolder)
	folders.PUT("/:id", RenameFolder)
	folders.DELETE("/:id", DeleteFolder)
	folders.POST("/:id/delete", DeleteFolder)
}
