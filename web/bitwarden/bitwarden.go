// Package bitwarden exposes an API compatible with the Bitwarden Open-Soure apps.
package bitwarden

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cozy/cozy-stack/model/bitwarden"
	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

// Prelogin tells to the client how many KDF iterations it must apply when
// hashing the master password.
func Prelogin(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	settings, err := settings.Get(inst)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, echo.Map{
		"Kdf":           settings.PassphraseKdf,
		"KdfIterations": settings.PassphraseKdfIterations,
	})
}

// GetProfile is the handler for the route to get profile information.
func GetProfile(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.GET, consts.BitwardenProfiles); err != nil {
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
		return err
	}
	return c.JSON(http.StatusOK, profile)
}

// UpdateProfile is the handler for the route to update the profile. Currently,
// only the hint for the master password can be changed.
func UpdateProfile(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.PUT, consts.BitwardenProfiles); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}

	var data struct {
		Hint string `json:"masterPasswordHint"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&data); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid JSON payload",
		})
	}
	settings, err := settings.Get(inst)
	if err != nil {
		return err
	}
	settings.PassphraseHint = data.Hint
	if err := settings.Save(inst); err != nil {
		return err
	}
	profile, err := newProfileResponse(inst, settings)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, profile)
}

// SetKeyPair is the handler for setting the key pair: public and private keys.
func SetKeyPair(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.POST, consts.BitwardenProfiles); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}

	var data struct {
		Private string `json:"encryptedPrivateKey"`
		Public  string `json:"publicKey"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&data); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid JSON payload",
		})
	}
	settings, err := settings.Get(inst)
	if err != nil {
		return err
	}
	if err := settings.SetKeyPair(inst, data.Public, data.Private); err != nil {
		return err
	}
	profile, err := newProfileResponse(inst, settings)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, profile)
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
			"error": "invalid JSON payload",
		})
	}

	if err := lifecycle.CheckPassphrase(inst, []byte(data.Hashed)); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid masterPasswordHash",
		})
	}

	settings, err := settings.Get(inst)
	if err != nil {
		return err
	}
	settings.SecurityStamp = lifecycle.NewSecurityStamp()
	if err := settings.Save(inst); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// GetRevisionDate returns the date of the last synchronization (as a number of
// milliseconds).
func GetRevisionDate(c echo.Context) error {
	pdoc, err := middlewares.GetPermission(c)
	if err != nil {
		return err
	}
	if pdoc.Type != permission.TypeOauth || pdoc.Client == nil {
		return permission.ErrInvalidAudience
	}
	client := pdoc.Client.(*oauth.Client)
	at, ok := client.SynchronizedAt.(time.Time)
	if !ok {
		if client.Metadata != nil {
			at = client.Metadata.CreatedAt
		} else {
			at = time.Now()
		}
	}
	milliseconds := fmt.Sprintf("%d", at.Nanosecond()/1000000)
	return c.Blob(http.StatusOK, "text/plain", []byte(milliseconds))
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
	settings, err := settings.Get(inst)
	if err != nil {
		return err
	}
	key := settings.Key

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
	settings, err := settings.Get(inst)
	if err != nil {
		return err
	}
	key := settings.Key

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
	api.GET("/sync", Sync)

	accounts := api.Group("/accounts")
	accounts.POST("/prelogin", Prelogin)
	accounts.GET("/profile", GetProfile)
	accounts.POST("/profile", UpdateProfile)
	accounts.PUT("/profile", UpdateProfile)
	accounts.POST("/keys", SetKeyPair)
	accounts.POST("/security-stamp", ChangeSecurityStamp)
	accounts.GET("/revision-date", GetRevisionDate)

	ciphers := api.Group("/ciphers")
	ciphers.GET("", ListCiphers)
	ciphers.POST("", CreateCipher)
	ciphers.GET("/:id", GetCipher)
	ciphers.POST("/:id", UpdateCipher)
	ciphers.PUT("/:id", UpdateCipher)
	ciphers.DELETE("/:id", DeleteCipher)
	ciphers.POST("/:id/delete", DeleteCipher)
	ciphers.POST("/:id/share", ShareCipher)
	ciphers.PUT("/:id/share", ShareCipher)

	folders := api.Group("/folders")
	folders.GET("", ListFolders)
	folders.POST("", CreateFolder)
	folders.GET("/:id", GetFolder)
	folders.POST("/:id", RenameFolder)
	folders.PUT("/:id", RenameFolder)
	folders.DELETE("/:id", DeleteFolder)
	folders.POST("/:id/delete", DeleteFolder)
}
