// Package bitwarden exposes an API compatible with the Bitwarden Open-Soure apps.
package bitwarden

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/bitwarden"
	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/session"
	"github.com/cozy/cozy-stack/pkg/config/config"
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
	setting, err := settings.Get(inst)
	if err != nil {
		return err
	}
	profile, err := newProfileResponse(inst, setting)
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
	setting, err := settings.Get(inst)
	if err != nil {
		return err
	}
	setting.PassphraseHint = data.Hint
	if err := setting.Save(inst); err != nil {
		return err
	}
	profile, err := newProfileResponse(inst, setting)
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
	setting, err := settings.Get(inst)
	if err != nil {
		return err
	}
	if err := setting.SetKeyPair(inst, data.Public, data.Private); err != nil {
		inst.Logger().WithField("nspace", "bitwarden").
			Infof("Cannot set key pair: %s", err)
		return err
	}
	profile, err := newProfileResponse(inst, setting)
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

	setting, err := settings.Get(inst)
	if err != nil {
		return err
	}
	setting.SecurityStamp = lifecycle.NewSecurityStamp()
	if err := setting.Save(inst); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// GetRevisionDate returns the date of the last synchronization (as a number of
// milliseconds).
func GetRevisionDate(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.GET, consts.BitwardenProfiles); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}
	setting, err := settings.Get(inst)
	if err != nil {
		return err
	}

	at := setting.Metadata.UpdatedAt
	milliseconds := fmt.Sprintf("%d", at.UnixNano()/1000000)
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

	if inst.HasAuthMode(instance.TwoFactorMail) {
		if !checkTwoFactor(c, inst) {
			return nil
		}
	}

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
	if err := session.SendNewRegistrationNotification(inst, client.ClientID); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

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
	setting, err := settings.Get(inst)
	if err != nil {
		return err
	}
	key := setting.Key

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

// checkTwoFactor returns true if the request has a valid 2FA code.
func checkTwoFactor(c echo.Context, inst *instance.Instance) bool {
	cache := config.GetConfig().CacheStorage
	key := "bw-2fa:" + inst.Domain

	if passcode := c.FormValue("twoFactorToken"); passcode != "" {
		if token, ok := cache.Get(key); ok {
			if inst.ValidateTwoFactorPasscode(token, passcode) {
				return true
			}
		}
	}

	// Allow the settings webapp get a bitwarden token without the 2FA. It's OK
	// from a security point of view as we still have 2 factors: the password
	// and a valid session cookie.
	if _, ok := middlewares.GetSession(c); ok {
		return true
	}

	email, err := inst.SettingsEMail()
	if err != nil {
		_ = c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
		return false
	}
	var obscured string
	if parts := strings.SplitN(email, "@", 2); len(parts) == 2 {
		s := strings.Map(func(_ rune) rune { return '*' }, parts[0])
		obscured = s + "@" + parts[1]
	}

	token, err := lifecycle.SendTwoFactorPasscode(inst)
	if err != nil {
		_ = c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
		return false
	}
	cache.Set(key, token, 5*time.Minute)

	_ = c.JSON(http.StatusBadRequest, echo.Map{
		"error":             "invalid_grant",
		"error_description": "Two factor required.",
		// 1 means email
		// https://github.com/bitwarden/jslib/blob/master/src/enums/twoFactorProviderType.ts
		"TwoFactorProviders":  []int{1},
		"TwoFactorProviders2": map[string]string{"1": obscured},
	})
	return false
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
	setting, err := settings.Get(inst)
	if err != nil {
		return err
	}
	key := setting.Key

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

// GetCozy returns the information about the cozy organization, including the
// organization key.
func GetCozy(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if err := middlewares.AllowWholeType(c, permission.GET, consts.BitwardenOrganizations); err != nil {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid token",
		})
	}

	setting, err := settings.Get(inst)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}
	orgKey, err := setting.OrganizationKey()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": err.Error(),
		})
	}

	res := map[string]interface{}{
		"organizationId":  setting.OrganizationID,
		"collectionId":    setting.CollectionID,
		"organizationKey": orgKey,
	}
	return c.JSON(http.StatusOK, res)
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

	settings := api.Group("/settings")
	settings.GET("/domains", GetDomains)
	settings.PUT("/domains", UpdateDomains)
	settings.POST("/domains", UpdateDomains)

	ciphers := api.Group("/ciphers")
	ciphers.GET("", ListCiphers)
	ciphers.POST("", CreateCipher)
	ciphers.POST("/create", CreateSharedCipher)
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

	hub := router.Group("/notifications/hub")
	hub.GET("", WebsocketHub)
	hub.POST("/negotiate", NegotiateHub)

	orgs := router.Group("/organizations")
	orgs.GET("/cozy", GetCozy)

	icons := router.Group("/icons")
	cacheControl := middlewares.CacheControl(middlewares.CacheOptions{
		MaxAge: 24 * time.Hour,
	})
	icons.GET("/:domain/icon.png", GetIcon, cacheControl)
}
