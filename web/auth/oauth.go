package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/move"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/session"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/model/vfs"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/pkg/registry"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

type webappParams struct {
	Name string
	Slug string
}

type authorizeParams struct {
	instance        *instance.Instance
	state           string
	clientID        string
	redirectURI     string
	scope           string
	resType         string
	challenge       string
	challengeMethod string
	client          *oauth.Client
	webapp          *webappParams
}

func checkAuthorizeParams(c echo.Context, params *authorizeParams) (bool, error) {
	if params.state == "" {
		return true, renderError(c, http.StatusBadRequest, "Error No state parameter")
	}
	if params.clientID == "" {
		return true, renderError(c, http.StatusBadRequest, "Error No client_id parameter")
	}
	if params.redirectURI == "" {
		return true, renderError(c, http.StatusBadRequest, "Error No redirect_uri parameter")
	}
	if params.resType != "code" {
		return true, renderError(c, http.StatusBadRequest, "Error Invalid response type")
	}
	if params.challenge != "" && params.challengeMethod != "S256" {
		return true, renderError(c, http.StatusBadRequest, "Error Invalid challenge code method")
	}
	if params.challengeMethod == "S256" && params.challenge == "" {
		return true, renderError(c, http.StatusBadRequest, "Error No challenge code")
	}

	client, err := oauth.FindClient(params.instance, params.clientID)
	if err != nil {
		return true, renderError(c, http.StatusBadRequest, "Error No registered client")
	}
	params.client = client
	if !params.client.AcceptRedirectURI(params.redirectURI) {
		return true, renderError(c, http.StatusBadRequest, "Error Incorrect redirect_uri")
	}

	params.scope = strings.TrimSpace(params.scope)
	if params.scope == "*" {
		instance := middlewares.GetInstance(c)
		context := instance.ContextName
		if context == "" {
			context = config.DefaultInstanceContext
		}
		cfg := config.GetConfig().Flagship.Contexts[context]
		skipCertification := false
		if cfg, ok := cfg.(map[string]interface{}); ok {
			skipCertification = cfg["skip_certification"] == true
		}
		if !skipCertification && !params.client.Flagship {
			return true, renderConfirmFlagship(c, params.clientID)
		}
		return false, nil
	}

	if appSlug := oauth.GetLinkedAppSlug(params.client.SoftwareID); appSlug != "" {
		webapp, err := registry.GetLatestVersion(appSlug, "stable", params.instance.Registries())

		if err != nil {
			return true, renderError(c, http.StatusBadRequest, "Cannot find application on instance registries")
		}

		var manifest struct {
			Slug        string         `json:"slug"`
			Name        string         `json:"name"`
			Permissions permission.Set `json:"permissions"`
		}
		err = json.Unmarshal(webapp.Manifest, &manifest)
		if err != nil {
			return true, renderError(c, http.StatusBadRequest, "Cannot decode application manifest")
		}

		params.scope, err = manifest.Permissions.MarshalScopeString()
		if err != nil {
			return true, renderError(c, http.StatusBadRequest, "Cannot marshal scope permissions")
		}

		params.webapp = &webappParams{
			Slug: manifest.Slug,
			Name: manifest.Name,
		}
	}

	if params.scope == "" {
		return true, renderError(c, http.StatusBadRequest, "Error No scope parameter")
	}
	if params.scope == oauth.ScopeLogin && !params.client.AllowLoginScope {
		return true, renderError(c, http.StatusBadRequest, "Error No scope parameter")
	}

	return false, nil
}

func authorizeForm(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	params := authorizeParams{
		instance:        instance,
		state:           c.QueryParam("state"),
		clientID:        c.QueryParam("client_id"),
		redirectURI:     c.QueryParam("redirect_uri"),
		scope:           c.QueryParam("scope"),
		resType:         c.QueryParam("response_type"),
		challenge:       c.QueryParam("code_challenge"),
		challengeMethod: c.QueryParam("code_challenge_method"),
	}

	if hasError, err := checkAuthorizeParams(c, &params); hasError {
		return err
	}

	if !middlewares.IsLoggedIn(c) {
		u := instance.PageURL("/auth/login", url.Values{
			"redirect": {instance.FromURL(c.Request().URL)},
		})
		return c.Redirect(http.StatusSeeOther, u)
	}

	// For a scope "login": such client is only used to transmit authentication
	// for the manager. It does not require any authorization from the user, and
	// generate a code without asking any permission.
	if params.scope == oauth.ScopeLogin {
		access, err := oauth.CreateAccessCode(params.instance, params.client, "" /* = scope */, "" /* = challenge */)
		if err != nil {
			return err
		}

		u, err := url.ParseRequestURI(params.redirectURI)
		if err != nil {
			return renderError(c, http.StatusBadRequest, "Error Invalid redirect_uri")
		}

		q := u.Query()
		// We should be sending "code" only, but for compatibility reason, we keep
		// the access_code parameter that we used to send in our first impl.
		q.Set("access_code", access.Code)
		q.Set("code", access.Code)
		q.Set("state", params.state)
		u.RawQuery = q.Encode()
		u.Fragment = ""

		return c.Redirect(http.StatusFound, u.String()+"#")
	}

	permissions, err := permission.UnmarshalScopeString(params.scope)
	if err != nil {
		context := instance.ContextName
		if context == "" {
			context = config.DefaultInstanceContext
		}
		cfg := config.GetConfig().Flagship.Contexts[context]
		skipCertification := false
		if cfg, ok := cfg.(map[string]interface{}); ok {
			skipCertification = cfg["skip_certification"] == true
		}
		if params.scope != "*" || (!skipCertification && !params.client.Flagship) {
			return renderError(c, http.StatusBadRequest, "Error Invalid scope")
		}
		permissions = permission.MaximalSet()
	}
	readOnly := true
	for _, p := range permissions {
		if !p.Verbs.ReadOnly() {
			readOnly = false
		}
	}
	params.client.ClientID = params.client.CouchID

	var clientDomain string
	clientURL, err := url.Parse(params.client.ClientURI)
	if err != nil {
		clientDomain = params.client.ClientURI
	} else {
		clientDomain = clientURL.Hostname()
	}

	// This Content-Security-Policy (CSP) nonce is here to allow the display of
	// logos for OAuth clients on the authorize page.
	if logoURI := params.client.LogoURI; logoURI != "" {
		logoURL, err := url.Parse(logoURI)
		if err == nil {
			csp := c.Response().Header().Get(echo.HeaderContentSecurityPolicy)
			if !strings.Contains(csp, "img-src") {
				c.Response().Header().Set(echo.HeaderContentSecurityPolicy,
					fmt.Sprintf("%simg-src 'self' https://%s;", csp, logoURL.Hostname()+logoURL.EscapedPath()))
			}
		}
	}

	slugname, instanceDomain := instance.SlugAndDomain()

	hasFallback := c.QueryParam("fallback_uri") != ""
	return c.Render(http.StatusOK, "authorize.html", echo.Map{
		"Domain":           instance.ContextualDomain(),
		"ContextName":      instance.ContextName,
		"Locale":           instance.Locale,
		"Title":            instance.TemplateTitle(),
		"Favicon":          middlewares.Favicon(instance),
		"InstanceSlugName": slugname,
		"InstanceDomain":   instanceDomain,
		"ClientDomain":     clientDomain,
		"Client":           params.client,
		"State":            params.state,
		"RedirectURI":      params.redirectURI,
		"Scope":            params.scope,
		"Challenge":        params.challenge,
		"ChallengeMethod":  params.challengeMethod,
		"Permissions":      permissions,
		"ReadOnly":         readOnly,
		"CSRF":             c.Get("csrf"),
		"HasFallback":      hasFallback,
		"Webapp":           params.webapp,
	})
}

func authorize(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	params := authorizeParams{
		instance:        instance,
		state:           c.FormValue("state"),
		clientID:        c.FormValue("client_id"),
		redirectURI:     c.FormValue("redirect_uri"),
		scope:           c.FormValue("scope"),
		resType:         c.FormValue("response_type"),
		challenge:       c.FormValue("challenge_code"),
		challengeMethod: c.FormValue("challenge_code_method"),
	}

	if hasError, err := checkAuthorizeParams(c, &params); hasError {
		return err
	}

	if !middlewares.IsLoggedIn(c) {
		return renderError(c, http.StatusUnauthorized, "Error Must be authenticated")
	}

	u, err := url.ParseRequestURI(params.redirectURI)
	if err != nil {
		return renderError(c, http.StatusBadRequest, "Error Invalid redirect_uri")
	}

	q := u.Query()
	q.Set("state", params.state)
	if params.client.OnboardingSecret != "" {
		q.Set("cozy_url", instance.Domain)
	}

	// Install the application in case of mobile client
	softwareID := params.client.SoftwareID
	if oauth.IsLinkedApp(softwareID) {
		manifest, err := GetLinkedApp(instance, softwareID)
		if err != nil {
			return err
		}
		slug := manifest.Slug()
		installer, err := app.NewInstaller(instance, app.Copier(consts.WebappType, instance), &app.InstallerOptions{
			Operation:  app.Install,
			Type:       consts.WebappType,
			SourceURL:  softwareID,
			Slug:       slug,
			Registries: instance.Registries(),
		})
		if err != app.ErrAlreadyExists {
			if err != nil {
				return err
			}
			go installer.Run()
		}
		params.scope = oauth.BuildLinkedAppScope(slug)
		if u.Scheme == "http" || u.Scheme == "https" {
			q.Set("fallback", instance.SubDomain(slug).String())
		}
	}

	access, err := oauth.CreateAccessCode(params.instance, params.client, params.scope, params.challenge)
	if err != nil {
		return err
	}
	var ip string
	if forwardedFor := c.Request().Header.Get(echo.HeaderXForwardedFor); forwardedFor != "" {
		ip = strings.TrimSpace(strings.SplitN(forwardedFor, ",", 2)[0])
	}
	if ip == "" {
		ip = strings.Split(c.Request().RemoteAddr, ":")[0]
	}
	instance.Logger().WithNamespace("loginaudit").
		Infof("Access code created from %s at %s with scope %s", ip, time.Now(), access.Scope)

	// We should be sending "code" only, but for compatibility reason, we keep
	// the access_code parameter that we used to send in our first impl.
	q.Set("access_code", access.Code)
	q.Set("code", access.Code)

	u.RawQuery = q.Encode()
	u.Fragment = ""
	location := u.String() + "#"

	wantsJSON := c.Request().Header.Get(echo.HeaderAccept) == echo.MIMEApplicationJSON
	if wantsJSON {
		return c.JSON(http.StatusOK, echo.Map{"deeplink": location})
	}
	return c.Redirect(http.StatusFound, location)
}

func renderConfirmFlagship(c echo.Context, clientID string) error {
	inst := middlewares.GetInstance(c)
	if !middlewares.IsLoggedIn(c) {
		u := inst.PageURL("/auth/login", url.Values{
			"redirect": {inst.FromURL(c.Request().URL)},
		})
		return c.Redirect(http.StatusSeeOther, u)
	}

	err := limits.CheckRateLimit(inst, limits.ConfirmFlagshipType)
	if limits.IsLimitReachedOrExceeded(err) {
		return renderError(c, http.StatusTooManyRequests, err.Error())
	}

	token, err := oauth.SendConfirmFlagshipCode(inst, clientID)
	if err != nil {
		return renderError(c, http.StatusInternalServerError, err.Error())
	}

	email, _ := inst.SettingsEMail()
	return c.Render(http.StatusOK, "confirm_flagship.html", echo.Map{
		"Domain":       inst.ContextualDomain(),
		"ContextName":  inst.ContextName,
		"Locale":       inst.Locale,
		"Title":        inst.TemplateTitle(),
		"Favicon":      middlewares.Favicon(inst),
		"Email":        email,
		"SupportEmail": inst.SupportEmailAddress(),
		"Token":        string(token),
		"ClientID":     clientID,
	})
}

type authorizeSharingParams struct {
	instance  *instance.Instance
	state     string
	sharingID string
}

func checkAuthorizeSharingParams(c echo.Context, params *authorizeSharingParams) (bool, error) {
	if params.state == "" {
		return true, renderError(c, http.StatusBadRequest, "Error No state parameter")
	}
	if params.sharingID == "" {
		return true, renderError(c, http.StatusBadRequest, "Error No sharing_id parameter")
	}
	return false, nil
}

func authorizeSharingForm(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	params := authorizeSharingParams{
		instance:  instance,
		state:     c.QueryParam("state"),
		sharingID: c.QueryParam("sharing_id"),
	}

	if hasError, err := checkAuthorizeSharingParams(c, &params); hasError {
		return err
	}

	if !middlewares.IsLoggedIn(c) {
		u := instance.PageURL("/auth/login", url.Values{
			"redirect": {instance.FromURL(c.Request().URL)},
		})
		return c.Redirect(http.StatusSeeOther, u)
	}

	s, err := sharing.FindSharing(instance, params.sharingID)
	if err != nil || s.Owner || s.Active || len(s.Members) < 2 {
		return renderError(c, http.StatusUnauthorized, "Error Invalid sharing")
	}

	hasShortcut := s.ShortcutID != ""
	var sharerDomain, targetType string
	sharerURL, err := url.Parse(s.Members[0].Instance)
	if err != nil {
		sharerDomain = s.Members[0].Instance
	} else {
		sharerDomain = sharerURL.Host
	}
	if s.Rules[0].DocType == consts.BitwardenOrganizations {
		targetType = instance.Translate("Notification Sharing Type Organization")
		hasShortcut = true
		s.Rules[0].Mime = "organization"
		if len(s.Rules) == 2 && s.Rules[1].DocType == consts.BitwardenCiphers {
			s.Rules = s.Rules[:1]
		}
	} else if s.Rules[0].DocType != consts.Files {
		targetType = instance.Translate("Notification Sharing Type Document")
	} else if s.Rules[0].Mime == "" {
		targetType = instance.Translate("Notification Sharing Type Directory")
	} else {
		targetType = instance.Translate("Notification Sharing Type File")
	}

	return c.Render(http.StatusOK, "authorize_sharing.html", echo.Map{
		"Domain":       instance.ContextualDomain(),
		"ContextName":  instance.ContextName,
		"Locale":       instance.Locale,
		"Title":        instance.TemplateTitle(),
		"Favicon":      middlewares.Favicon(instance),
		"SharerDomain": sharerDomain,
		"SharerName":   s.Members[0].PrimaryName(),
		"State":        params.state,
		"Sharing":      s,
		"CSRF":         c.Get("csrf"),
		"HasShortcut":  hasShortcut,
		"TargetType":   targetType,
	})
}

func authorizeSharing(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	params := authorizeSharingParams{
		instance:  instance,
		state:     c.FormValue("state"),
		sharingID: c.FormValue("sharing_id"),
	}

	if hasError, err := checkAuthorizeSharingParams(c, &params); hasError {
		return err
	}

	if !middlewares.IsLoggedIn(c) {
		return renderError(c, http.StatusUnauthorized, "Error Must be authenticated")
	}

	s, err := sharing.FindSharing(instance, params.sharingID)
	if err != nil {
		return err
	}
	if s.Owner || len(s.Members) < 2 {
		return sharing.ErrInvalidSharing
	}

	if c.FormValue("synchronize") == "" {
		if err = s.AddShortcut(instance, params.state); err != nil {
			return err
		}
		u := instance.SubDomain(consts.DriveSlug)
		u.RawQuery = "sharing=" + s.SID
		u.Fragment = "/folder/" + consts.SharedWithMeDirID
		return c.Redirect(http.StatusSeeOther, u.String())
	}

	if !s.Active {
		if err = s.SendAnswer(instance, params.state); err != nil {
			return err
		}
	}
	redirect := s.RedirectAfterAuthorizeURL(instance)
	return c.Redirect(http.StatusSeeOther, redirect.String())
}

func cancelAuthorizeSharing(c echo.Context) error {
	if !middlewares.IsLoggedIn(c) {
		return renderError(c, http.StatusUnauthorized, "Error Must be authenticated")
	}

	inst := middlewares.GetInstance(c)
	s, err := sharing.FindSharing(inst, c.Param("sharing-id"))
	if err != nil || s.Owner || len(s.Members) < 2 {
		return c.Redirect(http.StatusSeeOther, inst.SubDomain(consts.HomeSlug).String())
	}

	previewURL, err := s.GetPreviewURL(inst, c.QueryParam("state"))
	if err != nil {
		return c.Redirect(http.StatusSeeOther, inst.SubDomain(consts.HomeSlug).String())
	}
	return c.Redirect(http.StatusSeeOther, previewURL)
}

func authorizeMoveForm(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	state := c.QueryParam("state")
	if state == "" {
		return renderError(c, http.StatusBadRequest, "Error No state parameter")
	}
	clientID := c.QueryParam("client_id")
	if clientID == "" {
		return renderError(c, http.StatusBadRequest, "Error No client_id parameter")
	}
	redirectURI := c.QueryParam("redirect_uri")
	if redirectURI == "" {
		return renderError(c, http.StatusBadRequest, "Error No redirect_uri parameter")
	}
	client := oauth.Client{}
	if err := couchdb.GetDoc(inst, consts.OAuthClients, clientID, &client); err != nil {
		return renderError(c, http.StatusBadRequest, "Error No registered client")
	}
	if !client.AcceptRedirectURI(redirectURI) {
		return renderError(c, http.StatusBadRequest, "Error Incorrect redirect_uri")
	}

	if !inst.IsPasswordAuthenticationEnabled() {
		if !middlewares.IsLoggedIn(c) {
			u := c.Request().URL
			redirect := inst.PageURL(u.Path, u.Query())
			q := url.Values{"redirect": {redirect}}
			return c.Redirect(http.StatusSeeOther, inst.PageURL("/oidc/start", q))
		}
		twoFactorToken, err := lifecycle.SendTwoFactorPasscode(inst)
		if err != nil {
			return err
		}
		mail, _ := inst.SettingsEMail()
		return c.Render(http.StatusOK, "move_delegated_auth.html", echo.Map{
			"Domain":           inst.ContextualDomain(),
			"ContextName":      inst.ContextName,
			"Favicon":          middlewares.Favicon(inst),
			"TwoFactorToken":   string(twoFactorToken),
			"CredentialsError": "",
			"Email":            mail,
			"State":            state,
			"ClientID":         clientID,
			"Redirect":         redirectURI,
		})
	}

	publicName, err := inst.PublicName()
	if err != nil {
		publicName = ""
	}
	var title string
	if publicName == "" {
		title = inst.Translate("Login Welcome")
	} else {
		title = inst.Translate("Login Welcome name", publicName)
	}
	help := inst.Translate("Login Password help")
	iterations := 0
	if settings, err := settings.Get(inst); err == nil {
		iterations = settings.PassphraseKdfIterations
	}

	return c.Render(http.StatusOK, "authorize_move.html", echo.Map{
		"TemplateTitle":  inst.TemplateTitle(),
		"Domain":         inst.ContextualDomain(),
		"ContextName":    inst.ContextName,
		"Locale":         inst.Locale,
		"Iterations":     iterations,
		"Salt":           string(inst.PassphraseSalt()),
		"Title":          title,
		"PasswordHelp":   help,
		"CSRF":           c.Get("csrf"),
		"Favicon":        middlewares.Favicon(inst),
		"BottomNavBar":   middlewares.BottomNavigationBar(c),
		"CryptoPolyfill": middlewares.CryptoPolyfill(c),
		"State":          state,
		"ClientID":       clientID,
		"RedirectURI":    redirectURI,
	})
}

func authorizeMove(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if !inst.IsPasswordAuthenticationEnabled() {
		if !middlewares.IsLoggedIn(c) {
			return renderError(c, http.StatusUnauthorized, "Error Must be authenticated")
		}
		token := []byte(c.FormValue("two-factor-token"))
		passcode := c.FormValue("two-factor-passcode")
		correctPasscode := inst.ValidateTwoFactorPasscode(token, passcode)
		if !correctPasscode {
			errorMessage := inst.Translate(TwoFactorErrorKey)
			mail, _ := inst.SettingsEMail()
			return c.Render(http.StatusOK, "move_delegated_auth.html", echo.Map{
				"Domain":           inst.ContextualDomain(),
				"ContextName":      inst.ContextName,
				"Favicon":          middlewares.Favicon(inst),
				"TwoFactorToken":   string(token),
				"CredentialsError": errorMessage,
				"Email":            mail,
				"State":            c.FormValue("state"),
				"ClientID":         c.FormValue("client_id"),
				"Redirect":         c.FormValue("redirect"),
			})
		}
		u, err := moveSuccessURI(c)
		if err != nil {
			return err
		}
		return c.Redirect(http.StatusSeeOther, u)
	}

	// Check passphrase
	passphrase := []byte(c.FormValue("passphrase"))
	if lifecycle.CheckPassphrase(inst, passphrase) != nil {
		errorMessage := inst.Translate(CredentialsErrorKey)
		err := limits.CheckRateLimit(inst, limits.AuthType)
		if limits.IsLimitReachedOrExceeded(err) {
			if err = LoginRateExceeded(inst); err != nil {
				inst.Logger().WithNamespace("auth").Warn(err.Error())
			}
		}
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": errorMessage,
		})
	}

	if inst.HasAuthMode(instance.TwoFactorMail) && !isTrustedDevice(c, inst) {
		twoFactorToken, err := lifecycle.SendTwoFactorPasscode(inst)
		if err != nil {
			return err
		}
		v := url.Values{}
		v.Add("two_factor_token", string(twoFactorToken))
		v.Add("state", c.FormValue("state"))
		v.Add("client_id", c.FormValue("client_id"))
		v.Add("redirect", c.FormValue("redirect"))
		v.Add("trusted_device_checkbox", "false")

		return c.JSON(http.StatusOK, echo.Map{
			"redirect": inst.PageURL("/auth/twofactor", v),
		})
	}

	u, err := moveSuccessURI(c)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, echo.Map{
		"redirect": u,
	})
}

func moveSuccessURI(c echo.Context) (string, error) {
	u, err := url.Parse(c.FormValue("redirect"))
	if err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, "bad url: could not parse")
	}

	inst := middlewares.GetInstance(c)
	vault := settings.HasVault(inst)
	used, quota, err := DiskInfo(inst.VFS())
	if err != nil {
		return "", err
	}

	client, err := oauth.FindClient(inst, c.FormValue("client_id"))
	if err != nil {
		return "", err
	}
	access, err := oauth.CreateAccessCode(inst, client, move.MoveScope, "")
	if err != nil {
		return "", err
	}

	q := u.Query()
	q.Set("state", c.FormValue("state"))
	q.Set("code", access.Code)
	q.Set("vault", strconv.FormatBool(vault))
	q.Set("used", used)
	if quota != "" {
		q.Set("quota", quota)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// DiskInfo returns the used and quota disk space for the given VFS.
func DiskInfo(fs vfs.VFS) (string, string, error) {
	versions, err := fs.VersionsUsage()
	if err != nil {
		return "", "", err
	}
	files, err := fs.FilesUsage()
	if err != nil {
		return "", "", err
	}

	used := fmt.Sprintf("%d", files+versions)
	var quota string
	if q := fs.DiskQuota(); q > 0 {
		quota = fmt.Sprintf("%d", q)
	}
	return used, quota, nil
}

// AccessTokenReponse is the stuct used for serializing to JSON the response
// for an access token.
type AccessTokenReponse struct {
	Type    string `json:"token_type"`
	Scope   string `json:"scope"`
	Access  string `json:"access_token"`
	Refresh string `json:"refresh_token,omitempty"`
}

func accessToken(c echo.Context) error {
	grant := c.FormValue("grant_type")
	clientID := c.FormValue("client_id")
	clientSecret := c.FormValue("client_secret")
	verifier := c.FormValue("code_verifier")
	instance := middlewares.GetInstance(c)

	if grant == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "the grant_type parameter is mandatory",
		})
	}
	if clientID == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "the client_id parameter is mandatory",
		})
	}
	if clientSecret == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "the client_secret parameter is mandatory",
		})
	}

	client, err := oauth.FindClient(instance, clientID)
	if err != nil {
		if couchErr, isCouchErr := couchdb.IsCouchError(err); isCouchErr && couchErr.StatusCode >= 500 {
			return err
		}
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "the client must be registered",
		})
	}
	if subtle.ConstantTimeCompare([]byte(clientSecret), []byte(client.ClientSecret)) == 0 {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid client_secret",
		})
	}
	out := AccessTokenReponse{
		Type: "bearer",
	}

	slug := oauth.GetLinkedAppSlug(client.SoftwareID)
	if slug != "" {
		if err := CheckLinkedAppInstalled(instance, slug); err != nil {
			return err
		}
	}

	switch grant {
	case "authorization_code":
		code := c.FormValue("code")
		if code == "" {
			return c.JSON(http.StatusBadRequest, echo.Map{
				"error": "the code parameter is mandatory",
			})
		}
		accessCode := &oauth.AccessCode{}
		if err = couchdb.GetDoc(instance, consts.OAuthAccessCodes, code, accessCode); err != nil {
			return c.JSON(http.StatusBadRequest, echo.Map{
				"error": "invalid code",
			})
		}
		if accessCode.Challenge != "" {
			sum := sha256.Sum256([]byte(verifier))
			challenge := base64.RawURLEncoding.EncodeToString(sum[:])
			if challenge != accessCode.Challenge {
				return c.JSON(http.StatusBadRequest, echo.Map{
					"error": "invalid code_verifier",
				})
			}
		}
		out.Scope = accessCode.Scope
		out.Refresh, err = client.CreateJWT(instance, consts.RefreshTokenAudience, out.Scope)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, echo.Map{
				"error": "Can't generate refresh token",
			})
		}
		// Delete the access code, it can be used only once
		err = couchdb.DeleteDoc(instance, accessCode)
		if err != nil {
			instance.Logger().Errorf(
				"[oauth] Failed to delete the access code: %s", err)
		}

	case "refresh_token":
		token := c.FormValue("refresh_token")
		claims, ok := client.ValidToken(instance, consts.RefreshTokenAudience, token)
		if !ok && client.ClientKind == "sharing" {
			out.Refresh, claims, ok = sharing.TryTokenForMovedSharing(instance, client, token)
		}
		if !ok {
			return c.JSON(http.StatusBadRequest, echo.Map{
				"error": "invalid refresh token",
			})
		}
		// Code below is used to transform an old OAuth client token scope to
		// the new linked-app scope
		if slug != "" {
			out.Scope = oauth.BuildLinkedAppScope(slug)
		} else {
			out.Scope = claims.Scope
		}

	default:
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid grant type",
		})
	}

	out.Access, err = client.CreateJWT(instance, consts.AccessTokenAudience, out.Scope)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": "Can't generate access token",
		})
	}

	_ = session.RemoveLoginRegistration(instance.ContextualDomain(), clientID)
	return c.JSON(http.StatusOK, out)
}

// Used to trade a secret for OAuth client informations
func secretExchange(c echo.Context) error {
	type exchange struct {
		Secret string `json:"secret"`
	}
	e := new(exchange)

	instance := middlewares.GetInstance(c)
	err := json.NewDecoder(c.Request().Body).Decode(&e)
	if err != nil {
		return jsonapi.Errorf(http.StatusBadRequest, "%s", err)
	}

	if e.Secret == "" {
		return jsonapi.BadRequest(errors.New("Missing secret"))
	}

	doc, err := oauth.FindClientByOnBoardingSecret(instance, e.Secret)
	if err != nil {
		return jsonapi.NotFound(err)
	}

	if doc.OnboardingSecret == "" || doc.OnboardingSecret != e.Secret {
		return jsonapi.InvalidAttribute("secret", errors.New("Invalid secret"))
	}

	doc.TransformIDAndRev()
	return c.JSON(http.StatusOK, doc)
}

// CheckLinkedAppInstalled checks if a linked webapp has been installed to the
// instance
func CheckLinkedAppInstalled(inst *instance.Instance, slug string) error {
	_, err := app.GetWebappBySlugAndUpdate(inst, slug,
		app.Copier(consts.WebappType, inst), inst.Registries())
	if err == nil {
		return nil
	}

	const nbRetries = 10
	for i := 0; i < nbRetries; i++ {
		time.Sleep(3 * time.Second)
		if _, err := app.GetWebappBySlug(inst, slug); err == nil {
			return nil
		}
	}
	return fmt.Errorf("%s is not installed", slug)
}

// GetLinkedApp fetches the app manifest on the registry
func GetLinkedApp(instance *instance.Instance, softwareID string) (*app.WebappManifest, error) {
	var webappManifest app.WebappManifest
	appSlug := oauth.GetLinkedAppSlug(softwareID)
	webapp, err := registry.GetLatestVersion(appSlug, "stable", instance.Registries())
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(webapp.Manifest, &webappManifest)
	if err != nil {
		return nil, err
	}
	return &webappManifest, nil
}
