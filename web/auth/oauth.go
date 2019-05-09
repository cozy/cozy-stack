package auth

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/app"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/permission"
	"github.com/cozy/cozy-stack/model/session"
	"github.com/cozy/cozy-stack/model/sharing"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/pkg/registry"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

type webappParams struct {
	Name string
	Slug string
}

type authorizeParams struct {
	instance    *instance.Instance
	state       string
	clientID    string
	redirectURI string
	scope       string
	resType     string
	client      *oauth.Client
	webapp      *webappParams
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

	params.client = new(oauth.Client)
	if err := couchdb.GetDoc(params.instance, consts.OAuthClients, params.clientID, params.client); err != nil {
		return true, renderError(c, http.StatusBadRequest, "Error No registered client")
	}
	if !params.client.AcceptRedirectURI(params.redirectURI) {
		return true, renderError(c, http.StatusBadRequest, "Error Incorrect redirect_uri")
	}

	if IsLinkedApp(params.client.SoftwareID) {
		var webappManifest app.WebappManifest
		appSlug := GetLinkedAppSlug(params.client.SoftwareID)
		webapp, err := registry.GetLatestVersion(appSlug, "stable", params.instance.Registries())

		if err != nil {
			return true, renderError(c, http.StatusBadRequest, "Cannot find application on instance registries")
		}

		err = json.Unmarshal(webapp.Manifest, &webappManifest)
		if err != nil {
			return true, renderError(c, http.StatusBadRequest, "Cannot decode application manifest")
		}

		perms := webappManifest.Permissions()
		params.scope, err = perms.MarshalScopeString()
		if err != nil {
			return true, renderError(c, http.StatusBadRequest, "Cannot marshal scope permissions")
		}

		params.webapp = &webappParams{
			Slug: webappManifest.Slug(),
			Name: webappManifest.Name,
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
		instance:    instance,
		state:       c.QueryParam("state"),
		clientID:    c.QueryParam("client_id"),
		redirectURI: c.QueryParam("redirect_uri"),
		scope:       c.QueryParam("scope"),
		resType:     c.QueryParam("response_type"),
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
		access, err := oauth.CreateAccessCode(params.instance, params.clientID, "" /* = scope */)
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
		return renderError(c, http.StatusBadRequest, "Error Invalid scope")
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

	email, err := instance.SettingsEMail()
	if err != nil {
		email = instance.ContextualDomain()
	}
	hasFallback := c.QueryParam("fallback_uri") != ""
	return c.Render(http.StatusOK, "authorize.html", echo.Map{
		"Title":        instance.TemplateTitle(),
		"CozyUI":       middlewares.CozyUI(instance),
		"ThemeCSS":     middlewares.ThemeCSS(instance),
		"Domain":       instance.ContextualDomain(),
		"Email":        email,
		"ContextName":  instance.ContextName,
		"ClientDomain": clientDomain,
		"Locale":       instance.Locale,
		"Client":       params.client,
		"State":        params.state,
		"RedirectURI":  params.redirectURI,
		"Scope":        params.scope,
		"Permissions":  permissions,
		"ReadOnly":     readOnly,
		"CSRF":         c.Get("csrf"),
		"HasFallback":  hasFallback,
		"Webapp":       params.webapp,
	})
}

func authorize(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	params := authorizeParams{
		instance:    instance,
		state:       c.FormValue("state"),
		clientID:    c.FormValue("client_id"),
		redirectURI: c.FormValue("redirect_uri"),
		scope:       c.FormValue("scope"),
		resType:     c.FormValue("response_type"),
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
	if IsLinkedApp(softwareID) {
		manifest, err := GetLinkedApp(instance, softwareID)
		if err != nil {
			return err
		}
		slug := manifest.Slug()
		installer, err := app.NewInstaller(instance, instance.AppsCopier(consts.WebappType), &app.InstallerOptions{
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
		params.scope = BuildLinkedAppScope(slug)
		if u.Scheme == "http" || u.Scheme == "https" {
			q.Set("fallback", instance.SubDomain(slug).String())
		}
	}

	access, err := oauth.CreateAccessCode(params.instance, params.clientID, params.scope)
	if err != nil {
		return err
	}
	// We should be sending "code" only, but for compatibility reason, we keep
	// the access_code parameter that we used to send in our first impl.
	q.Set("access_code", access.Code)
	q.Set("code", access.Code)

	u.RawQuery = q.Encode()
	u.Fragment = ""
	location := u.String() + "#"

	wantsJSON := c.Request().Header.Get("Accept") == "application/json"
	if wantsJSON {
		return c.JSON(http.StatusOK, echo.Map{"deeplink": location})
	}
	return c.Redirect(http.StatusFound, location)
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

	var sharerDomain string
	sharerURL, err := url.Parse(s.Members[0].Instance)
	if err != nil {
		sharerDomain = s.Members[0].Instance
	} else {
		sharerDomain = sharerURL.Host
	}

	return c.Render(http.StatusOK, "authorize_sharing.html", echo.Map{
		"Title":        instance.TemplateTitle(),
		"CozyUI":       middlewares.CozyUI(instance),
		"ThemeCSS":     middlewares.ThemeCSS(instance),
		"Domain":       instance.ContextualDomain(),
		"ContextName":  instance.ContextName,
		"Locale":       instance.Locale,
		"SharerDomain": sharerDomain,
		"SharerName":   s.Members[0].PrimaryName(),
		"State":        params.state,
		"Sharing":      s,
		"CSRF":         c.Get("csrf"),
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

	if !s.Active {
		if err = s.SendAnswer(instance, params.state); err != nil {
			return err
		}
	}
	redirect := s.RedirectAfterAuthorizeURL(instance)
	return c.Redirect(http.StatusSeeOther, redirect.String())
}

func authorizeAppForm(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	if !middlewares.IsLoggedIn(c) {
		u := instance.PageURL("/auth/login", url.Values{
			"redirect": {instance.FromURL(c.Request().URL)},
		})
		return c.Redirect(http.StatusSeeOther, u)
	}

	app, ok, err := getApp(c, instance, c.QueryParam("slug"))
	if !ok || err != nil {
		return err
	}

	permissions := app.Permissions()
	return c.Render(http.StatusOK, "authorize_app.html", echo.Map{
		"Title":       instance.TemplateTitle(),
		"ThemeCSS":    middlewares.ThemeCSS(instance),
		"Domain":      instance.ContextualDomain(),
		"ContextName": instance.ContextName,
		"Slug":        app.Slug(),
		"Permissions": permissions,
		"CSRF":        c.Get("csrf"),
	})
}

func authorizeApp(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	if !middlewares.IsLoggedIn(c) {
		return renderError(c, http.StatusUnauthorized, "Error Must be authenticated")
	}

	a, ok, err := getApp(c, instance, c.FormValue("slug"))
	if !ok || err != nil {
		return err
	}

	a.SetState(app.Ready)
	err = a.Update(instance, nil)
	if err != nil {
		msg := instance.Translate("Could not activate application: %s", err.Error())
		return renderError(c, http.StatusUnauthorized, msg)
	}

	u := instance.SubDomain(a.Slug())
	return c.Redirect(http.StatusFound, u.String()+"#")
}

func getApp(c echo.Context, instance *instance.Instance, slug string) (app.Manifest, bool, error) {
	a, err := app.GetWebappBySlug(instance, slug)
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			return nil, false, renderError(c, http.StatusNotFound,
				`Application should have state "installed"`)
		}
		return nil, false, renderError(c, http.StatusInternalServerError,
			instance.Translate("Could not fetch application: %s", err.Error()))
	}
	if a.State() != app.Installed {
		return nil, false, renderError(c, http.StatusExpectationFailed,
			`Application should have state "installed"`)
	}
	return a, true, nil
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
	instance := middlewares.GetInstance(c)
	var slug string

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

	if IsLinkedApp(client.SoftwareID) {
		slug = GetLinkedAppSlug(client.SoftwareID)
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
		claims, ok := client.ValidToken(instance, consts.RefreshTokenAudience, c.FormValue("refresh_token"))
		if !ok {
			return c.JSON(http.StatusBadRequest, echo.Map{
				"error": "invalid refresh token",
			})
		}
		// Code below is used to transform an old OAuth client token scope to
		// the new linked-app scope
		if slug != "" {
			out.Scope = BuildLinkedAppScope(slug)
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
func CheckLinkedAppInstalled(instance *instance.Instance, slug string) error {
	i := 0
	for {
		i++
		_, err := app.GetWebappBySlug(instance, slug)
		if err == nil {
			return nil
		}
		if i == 10 {
			return fmt.Errorf("%s is not installed", slug)
		}
		time.Sleep(3 * time.Second)
	}
}

// GetLinkedAppSlug returns a linked app slug from a softwareID
func GetLinkedAppSlug(softwareID string) string {
	return strings.TrimPrefix(softwareID, "registry://")
}

// BuildLinkedAppScope returns a formatted scope for a linked app
func BuildLinkedAppScope(slug string) string {
	return fmt.Sprintf("@%s/%s", consts.Apps, slug)
}

// GetLinkedApp fetches the app manifest on the registry
func GetLinkedApp(instance *instance.Instance, softwareID string) (*app.WebappManifest, error) {
	var webappManifest app.WebappManifest
	appSlug := GetLinkedAppSlug(softwareID)
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

// IsLinkedApp checks if an OAuth client has a linked app
func IsLinkedApp(softwareID string) bool {
	return strings.HasPrefix(softwareID, "registry://")
}
