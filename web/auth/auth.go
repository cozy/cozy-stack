// Package auth provides register and login handlers
package auth

import (
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/sessions"
	"github.com/cozy/cozy-stack/pkg/sharing"
	"github.com/cozy/cozy-stack/pkg/utils"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
	"github.com/cozy/echo/middleware"
)

const (
	// CredentialsErrorKey is the key for translating the message showed to the
	// user when he/she enters incorrect credentials
	CredentialsErrorKey = "Login Credentials error"
	// TwoFactorErrorKey is the key for translating the message showed to the
	// user when he/she enters incorrect two factor secret
	TwoFactorErrorKey = "Login Two factor error"
)

// Home is the handler for /
// It redirects to the login page is the user is not yet authentified
// Else, it redirects to its home application (or onboarding)
func Home(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	if session, ok := middlewares.GetSession(c); ok {
		redirect := instance.DefaultRedirection()
		redirect = addCodeToRedirect(redirect, instance.ContextualDomain(), session.ID())
		cookie, err := session.ToCookie()
		if err != nil {
			return err
		}
		c.SetCookie(cookie)
		return c.Redirect(http.StatusSeeOther, redirect.String())
	}

	if len(instance.RegisterToken) > 0 {
		if !middlewares.CheckRegisterToken(c, instance) {
			return c.Render(http.StatusOK, "need_onboarding.html", echo.Map{
				"Domain": instance.ContextualDomain(),
				"Locale": instance.Locale,
			})
		}
		redirect := instance.SubDomain(consts.OnboardingSlug)
		redirect.RawQuery = c.Request().URL.RawQuery
		return c.Redirect(http.StatusSeeOther, redirect.String())
	}

	return c.Redirect(http.StatusSeeOther, instance.PageURL("/auth/login", nil))
}

// With the nested subdomains structure, the cookie set on the main domain can
// also be used to authentify the user on the apps subdomain. But with the flat
// subdomains structure, a new cookie is needed. To transfer the session, we
// add a code parameter to the redirect URL that can be exchanged to the
// cookie. The code can be used only once, is valid only one minute, and is
// specific to the app (it can't be used by another app).
func addCodeToRedirect(redirect *url.URL, domain, sessionID string) *url.URL {
	// TODO add rate-limiting on the number of session codes generated
	if config.GetConfig().Subdomains == config.FlatSubdomains {
		redirect = utils.CloneURL(redirect)
		if redirect.Host != domain {
			q := redirect.Query()
			q.Set("code", sessions.BuildCode(sessionID, redirect.Host).Value)
			redirect.RawQuery = q.Encode()
			return redirect
		}
	}
	return redirect
}

// SetCookieForNewSession creates a new session and sets the cookie on echo context
func SetCookieForNewSession(c echo.Context, longRunSession bool) (string, error) {
	instance := middlewares.GetInstance(c)
	session, err := sessions.New(instance, longRunSession)
	if err != nil {
		return "", err
	}
	cookie, err := session.ToCookie()
	if err != nil {
		return "", err
	}
	c.SetCookie(cookie)
	return session.ID(), nil
}

func renderLoginForm(c echo.Context, i *instance.Instance, code int, credsErrors string, redirect *url.URL) error {
	var title, help string

	publicName, err := i.PublicName()
	if err != nil {
		publicName = ""
	}

	redirectStr := redirect.String()
	redirectQuery := redirect.Query()

	var clientScope string
	if clientScopes := redirectQuery["scope"]; len(clientScopes) > 0 {
		clientScope = clientScopes[0]
	}

	if c.QueryParam("msg") == "passphrase-reset-requested" {
		title = i.Translate("Login Connect after reset requested title")
		help = i.Translate("Login Connect after reset requested help")
	} else if strings.Contains(redirectStr, "reconnect") {
		title = i.Translate("Login Reconnect title")
		help = i.Translate("Login Reconnect help")
	} else if i.HasDomain(redirect.Host) && redirect.Path == "/auth/authorize" && clientScope != oauth.ScopeLogin {
		title = i.Translate("Login Connect from oauth title")
		help = i.Translate("Login Connect from oauth help")
	} else if i.HasDomain(redirect.Host) && redirect.Path == "/auth/authorize/sharing" {
		title = i.Translate("Login Connect from sharing title", publicName)
		help = i.Translate("Login Connect from sharing help")
	} else {
		if publicName == "" {
			title = i.Translate("Login Welcome")
		} else {
			title = i.Translate("Login Welcome name", publicName)
		}
		help = i.Translate("Login Password help")
	}

	return c.Render(code, "login.html", echo.Map{
		"Domain":           i.ContextualDomain(),
		"Locale":           i.Locale,
		"Title":            title,
		"PasswordHelp":     help,
		"CredentialsError": credsErrors,
		"Redirect":         redirectStr,
		"TwoFactorForm":    false,
		"TwoFactorToken":   "",
		"CSRF":             c.Get("csrf"),
	})
}

func renderTwoFactorForm(c echo.Context, i *instance.Instance, code int, redirect *url.URL, twoFactorToken []byte, longRunSession bool) error {
	var title string
	publicName, err := i.PublicName()
	if err != nil {
		publicName = ""
	}
	if publicName == "" {
		title = i.Translate("Login Welcome")
	} else {
		title = i.Translate("Login Welcome name", publicName)
	}
	return c.Render(code, "login.html", echo.Map{
		"Domain":           i.ContextualDomain(),
		"Locale":           i.Locale,
		"Title":            title,
		"PasswordHelp":     "",
		"CredentialsError": nil,
		"Redirect":         redirect.String(),
		"LongRunSession":   longRunSession,
		"TwoFactorForm":    true,
		"TwoFactorToken":   string(twoFactorToken),
		"CSRF":             c.Get("csrf"),
	})
}

func loginForm(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	redirect, err := checkRedirectParam(c, instance.DefaultRedirection())
	if err != nil {
		return err
	}

	session, ok := middlewares.GetSession(c)
	if ok {
		redirect = addCodeToRedirect(redirect, instance.ContextualDomain(), session.ID())
		cookie, err := session.ToCookie()
		if err != nil {
			return err
		}
		c.SetCookie(cookie)
		return c.Redirect(http.StatusSeeOther, redirect.String())
	}

	return renderLoginForm(c, instance, http.StatusOK, "", redirect)
}

func login(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	wantsJSON := c.Request().Header.Get("Accept") == "application/json"

	redirect, err := checkRedirectParam(c, inst.DefaultRedirection())
	if err != nil {
		return err
	}

	successfulAuthentication := false
	twoFactorToken := []byte(c.FormValue("two-factor-token"))
	twoFactorPasscode := c.FormValue("two-factor-passcode")
	twoFactorTrustedDeviceToken := []byte(c.FormValue("two-factor-trusted-device-token"))
	twoFactorGenerateTrustedDeviceToken, _ := strconv.ParseBool(c.FormValue("two-factor-generate-trusted-device-token"))
	passphrase := []byte(c.FormValue("passphrase"))
	longRunSession, _ := strconv.ParseBool(c.FormValue("long-run-session"))

	var twoFactorGeneratedTrustedDeviceToken []byte

	twoFactorRequest := len(twoFactorToken) > 0 && twoFactorPasscode != ""
	passphraseRequest := len(passphrase) > 0

	var sessionID string
	session, ok := middlewares.GetSession(c)
	if ok {
		sessionID = session.ID()
	} else if twoFactorRequest {
		successfulAuthentication = inst.ValidateTwoFactorPasscode(
			twoFactorToken, twoFactorPasscode)

		if successfulAuthentication && twoFactorGenerateTrustedDeviceToken {
			twoFactorGeneratedTrustedDeviceToken, _ =
				inst.GenerateTwoFactorTrustedDeviceSecret(c.Request())
		}
	} else if passphraseRequest {
		if inst.CheckPassphrase(passphrase) == nil {
			switch {
			// In case the second factor authentication mode is "mail", we also
			// check that the mail has been confirmed. If not, 2FA is not actived.
			case inst.HasAuthMode(instance.TwoFactorMail):
				if len(twoFactorTrustedDeviceToken) > 0 {
					successfulAuthentication = inst.ValidateTwoFactorTrustedDeviceSecret(
						c.Request(), twoFactorTrustedDeviceToken)
				}
				if !successfulAuthentication {
					twoFactorToken, err = inst.SendTwoFactorPasscode()
					if err != nil {
						return err
					}
					if wantsJSON {
						return c.JSON(http.StatusOK, echo.Map{
							"redirect":         redirect.String(),
							"two_factor_token": string(twoFactorToken),
						})
					}
					return renderTwoFactorForm(c, inst, http.StatusOK, redirect, twoFactorToken, longRunSession)
				}
			default:
				successfulAuthentication = true
			}
		}
	}

	if successfulAuthentication {
		if sessionID, err = SetCookieForNewSession(c, longRunSession); err != nil {
			return err
		}

		var clientID string
		if inst.HasDomain(redirect.Host) && redirect.Path == "/auth/authorize" {
			clientID = redirect.Query().Get("client_id")
		}

		if err = sessions.StoreNewLoginEntry(inst, sessionID, clientID, c.Request(), true); err != nil {
			inst.Logger().Errorf("Could not store session history %q: %s", sessionID, err)
		}
	}

	// not logged-in
	if sessionID == "" {
		var errorMessage string
		if twoFactorRequest {
			errorMessage = inst.Translate(TwoFactorErrorKey)
		} else {
			errorMessage = inst.Translate(CredentialsErrorKey)
		}
		if wantsJSON {
			return c.JSON(http.StatusUnauthorized, echo.Map{
				"error": errorMessage,
			})
		}
		return renderLoginForm(c, inst, http.StatusUnauthorized,
			errorMessage, redirect)
	}

	// logged-in
	redirect = addCodeToRedirect(redirect, inst.ContextualDomain(), sessionID)
	if wantsJSON {
		result := echo.Map{"redirect": redirect.String()}
		if len(twoFactorGeneratedTrustedDeviceToken) > 0 {
			result["two_factor_trusted_device_token"] = string(twoFactorGeneratedTrustedDeviceToken)
		}
		return c.JSON(http.StatusOK, result)
	}

	return c.Redirect(http.StatusSeeOther, redirect.String())
}

func logout(c echo.Context) error {
	res := c.Response()
	origin := c.Request().Header.Get(echo.HeaderOrigin)
	res.Header().Set(echo.HeaderAccessControlAllowOrigin, origin)
	res.Header().Set(echo.HeaderAccessControlAllowCredentials, "true")

	instance := middlewares.GetInstance(c)
	if !middlewares.AllowLogout(c) {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "The user can logout only from client-side apps",
		})
	}

	session, ok := middlewares.GetSession(c)
	if ok {
		c.SetCookie(session.Delete(instance))
	}

	return c.NoContent(http.StatusNoContent)
}

func logoutOthers(c echo.Context) error {
	res := c.Response()
	origin := c.Request().Header.Get(echo.HeaderOrigin)
	res.Header().Set(echo.HeaderAccessControlAllowOrigin, origin)
	res.Header().Set(echo.HeaderAccessControlAllowCredentials, "true")

	instance := middlewares.GetInstance(c)
	if !middlewares.AllowLogout(c) {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "The user can logout only from client-side apps",
		})
	}

	session, ok := middlewares.GetSession(c)
	if !ok {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "Could not retrieve session",
		})
	}
	if err := sessions.DeleteOthers(instance, session.ID()); err != nil {
		return err
	}

	return c.NoContent(http.StatusNoContent)
}

func logoutPreflight(c echo.Context) error {
	req := c.Request()
	res := c.Response()
	origin := req.Header.Get(echo.HeaderOrigin)

	res.Header().Add(echo.HeaderVary, echo.HeaderOrigin)
	res.Header().Add(echo.HeaderVary, echo.HeaderAccessControlRequestMethod)
	res.Header().Add(echo.HeaderVary, echo.HeaderAccessControlRequestHeaders)
	res.Header().Set(echo.HeaderAccessControlAllowOrigin, origin)
	res.Header().Set(echo.HeaderAccessControlAllowMethods, echo.DELETE)
	res.Header().Set(echo.HeaderAccessControlAllowCredentials, "true")
	res.Header().Set(echo.HeaderAccessControlMaxAge, middlewares.MaxAgeCORS)
	if h := req.Header.Get(echo.HeaderAccessControlRequestHeaders); h != "" {
		res.Header().Set(echo.HeaderAccessControlAllowHeaders, h)
	}

	return c.NoContent(http.StatusNoContent)
}

// checkRedirectParam returns the optional redirect query parameter. If not
// empty, we check that the redirect is a subdomain of the cozy-instance.
func checkRedirectParam(c echo.Context, defaultRedirect *url.URL) (*url.URL, error) {
	redirect := c.FormValue("redirect")
	if redirect == "" {
		redirect = defaultRedirect.String()
	}

	u, err := url.Parse(redirect)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest,
			"bad url: could not parse")
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, echo.NewHTTPError(http.StatusBadRequest,
			"bad url: bad scheme")
	}

	instance := middlewares.GetInstance(c)
	if !instance.HasDomain(u.Host) {
		instanceHost, appSlug, _ := middlewares.SplitHost(u.Host)
		if !instance.HasDomain(instanceHost) || appSlug == "" {
			return nil, echo.NewHTTPError(http.StatusBadRequest,
				"bad url: should be subdomain")
		}
		return u, nil
	}

	// To protect against stealing authorization code with redirection, the
	// fragment is always overridden. Most browsers keep URI fragments upon
	// redirects, to make sure to override them, we put an empty one.
	//
	// see: oauthsecurity.com/#provider-in-the-middle
	// see: 7.4.2 OAuth2 in Action
	u.Fragment = "="
	return u, nil
}

func registerClient(c echo.Context) error {
	// TODO add rate-limiting to prevent DOS attacks
	client := new(oauth.Client)
	if err := json.NewDecoder(c.Request().Body).Decode(client); err != nil {
		return err
	}
	instance := middlewares.GetInstance(c)
	// We do not allow the creation of clients allowed to have an empty scope
	// ("login" scope), except via the CLI.
	if client.AllowLoginScope {
		perm, err := middlewares.GetPermission(c)
		if err != nil {
			return err
		}
		if perm.Type != permissions.TypeCLI {
			return echo.NewHTTPError(http.StatusUnauthorized,
				"Not authorized to create client with given parameters")
		}
	}
	if err := client.Create(instance); err != nil {
		return c.JSON(err.Code, err)
	}
	return c.JSON(http.StatusCreated, client)
}

func readClient(c echo.Context) error {
	client := c.Get("client").(*oauth.Client)
	client.TransformIDAndRev()
	return c.JSON(http.StatusOK, client)
}

func updateClient(c echo.Context) error {
	// TODO add rate-limiting to prevent DOS attacks
	client := new(oauth.Client)
	if err := json.NewDecoder(c.Request().Body).Decode(client); err != nil {
		return err
	}
	oldClient := c.Get("client").(*oauth.Client)
	instance := middlewares.GetInstance(c)
	if err := client.Update(instance, oldClient); err != nil {
		return c.JSON(err.Code, err)
	}
	return c.JSON(http.StatusOK, client)
}

func deleteClient(c echo.Context) error {
	client := c.Get("client").(*oauth.Client)
	instance := middlewares.GetInstance(c)
	if err := client.Delete(instance); err != nil {
		return c.JSON(err.Code, err)
	}
	return c.NoContent(http.StatusNoContent)
}

type authorizeParams struct {
	instance    *instance.Instance
	state       string
	clientID    string
	redirectURI string
	scope       string
	resType     string
	client      *oauth.Client
}

func checkAuthorizeParams(c echo.Context, params *authorizeParams) (bool, error) {
	if params.state == "" {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Domain": params.instance.ContextualDomain(),
			"Error":  "Error No state parameter",
		})
	}
	if params.clientID == "" {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Domain": params.instance.ContextualDomain(),
			"Error":  "Error No client_id parameter",
		})
	}
	if params.redirectURI == "" {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Domain": params.instance.ContextualDomain(),
			"Error":  "Error No redirect_uri parameter",
		})
	}
	if params.resType != "code" {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Domain": params.instance.ContextualDomain(),
			"Error":  "Error Invalid response type",
		})
	}
	if params.scope == "" {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Domain": params.instance.ContextualDomain(),
			"Error":  "Error No scope parameter",
		})
	}

	params.client = new(oauth.Client)
	if err := couchdb.GetDoc(params.instance, consts.OAuthClients, params.clientID, params.client); err != nil {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Domain": params.instance.ContextualDomain(),
			"Error":  "Error No registered client",
		})
	}
	if !params.client.AcceptRedirectURI(params.redirectURI) {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Domain": params.instance.ContextualDomain(),
			"Error":  "Error Incorrect redirect_uri",
		})
	}
	if params.scope == oauth.ScopeLogin && !params.client.AllowLoginScope {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Domain": params.instance.ContextualDomain(),
			"Error":  "Error No scope parameter",
		})
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
			return c.Render(http.StatusBadRequest, "error.html", echo.Map{
				"Domain": instance.ContextualDomain(),
				"Error":  "Error Invalid redirect_uri",
			})
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

	permissions, err := permissions.UnmarshalScopeString(params.scope)
	if err != nil {
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Domain": instance.ContextualDomain(),
			"Error":  "Error Invalid scope",
		})
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

	return c.Render(http.StatusOK, "authorize.html", echo.Map{
		"Domain":       instance.ContextualDomain(),
		"ClientDomain": clientDomain,
		"Locale":       instance.Locale,
		"Client":       params.client,
		"State":        params.state,
		"RedirectURI":  params.redirectURI,
		"Scope":        params.scope,
		"Permissions":  permissions,
		"ReadOnly":     readOnly,
		"CSRF":         c.Get("csrf"),
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
		return c.Render(http.StatusUnauthorized, "error.html", echo.Map{
			"Domain": instance.ContextualDomain(),
			"Error":  "Error Must be authenticated",
		})
	}

	u, err := url.ParseRequestURI(params.redirectURI)
	if err != nil {
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Domain": instance.ContextualDomain(),
			"Error":  "Error Invalid redirect_uri",
		})
	}

	access, err := oauth.CreateAccessCode(params.instance, params.clientID, params.scope)
	if err != nil {
		return err
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

type authorizeSharingParams struct {
	instance  *instance.Instance
	state     string
	sharingID string
}

func checkAuthorizeSharingParams(c echo.Context, params *authorizeSharingParams) (bool, error) {
	if params.state == "" {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Domain": params.instance.ContextualDomain(),
			"Error":  "Error No state parameter",
		})
	}
	if params.sharingID == "" {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Domain": params.instance.ContextualDomain(),
			"Error":  "Error No sharing_id parameter",
		})
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
		return c.Render(http.StatusUnauthorized, "error.html", echo.Map{
			"Domain": instance.ContextualDomain(),
			"Error":  "Error Invalid sharing",
		})
	}

	var sharerDomain string
	sharerURL, err := url.Parse(s.Members[0].Instance)
	if err != nil {
		sharerDomain = s.Members[0].Instance
	} else {
		sharerDomain = sharerURL.Host
	}

	return c.Render(http.StatusOK, "authorize_sharing.html", echo.Map{
		"Locale":       instance.Locale,
		"Domain":       instance.ContextualDomain(),
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
		return c.Render(http.StatusUnauthorized, "error.html", echo.Map{
			"Domain": instance.ContextualDomain(),
			"Error":  "Error Must be authenticated",
		})
	}

	s, err := sharing.FindSharing(instance, params.sharingID)
	if err != nil {
		return err
	}
	if s.Owner || s.Active || len(s.Members) < 2 {
		return sharing.ErrInvalidSharing
	}

	if err = s.SendAnswer(instance, params.state); err != nil {
		return err
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
		"Domain":      instance.ContextualDomain(),
		"Slug":        app.Slug(),
		"Permissions": permissions,
		"CSRF":        c.Get("csrf"),
	})
}

func authorizeApp(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	if !middlewares.IsLoggedIn(c) {
		return c.Render(http.StatusUnauthorized, "error.html", echo.Map{
			"Domain": instance.ContextualDomain(),
			"Error":  "Error Must be authenticated",
		})
	}

	app, ok, err := getApp(c, instance, c.FormValue("slug"))
	if !ok || err != nil {
		return err
	}

	app.SetState(apps.Ready)
	err = app.Update(instance)
	if err != nil {
		return c.Render(http.StatusInternalServerError, "error.html", echo.Map{
			"Domain": instance.ContextualDomain(),
			"Error":  instance.Translate("Could not activate application: %s", err.Error()),
		})
	}

	u := instance.SubDomain(app.Slug())
	return c.Redirect(http.StatusFound, u.String()+"#")
}

func getApp(c echo.Context, instance *instance.Instance, slug string) (apps.Manifest, bool, error) {
	app, err := apps.GetWebappBySlug(instance, slug)
	if err != nil {
		if couchdb.IsNotFoundError(err) {
			return nil, false, c.Render(http.StatusNotFound, "error.html", echo.Map{
				"Domain": instance.ContextualDomain(),
				"Error":  `Application should have state "installed"`,
			})
		}
		return nil, false, c.Render(http.StatusInternalServerError, "error.html", echo.Map{
			"Domain": instance.ContextualDomain(),
			"Error":  instance.Translate("Could not fetch application: %s", err.Error()),
		})
	}
	if app.State() != apps.Installed {
		return nil, false, c.Render(http.StatusExpectationFailed, "error.html", echo.Map{
			"Domain": instance.ContextualDomain(),
			"Error":  `Application should have state "installed"`,
		})
	}
	return app, true, nil
}

type accessTokenReponse struct {
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
	out := accessTokenReponse{
		Type: "bearer",
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
		out.Refresh, err = client.CreateJWT(instance, permissions.RefreshTokenAudience, out.Scope)
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
		claims, ok := client.ValidToken(instance, permissions.RefreshTokenAudience, c.FormValue("refresh_token"))
		if !ok {
			return c.JSON(http.StatusBadRequest, echo.Map{
				"error": "invalid refresh token",
			})
		}
		out.Scope = claims.Scope

	default:
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid grant type",
		})
	}

	out.Access, err = client.CreateJWT(instance, permissions.AccessTokenAudience, out.Scope)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": "Can't generate access token",
		})
	}

	sessions.RemoveLoginRegistration(instance.ContextualDomain(), clientID)
	return c.JSON(http.StatusOK, out)
}

func checkRegistrationToken(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		header := c.Request().Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			return c.JSON(http.StatusBadRequest, echo.Map{
				"error": "invalid_token",
			})
		}
		instance := middlewares.GetInstance(c)
		client, err := oauth.FindClient(instance, c.Param("client-id"))
		if err != nil {
			return c.JSON(http.StatusNotFound, echo.Map{
				"error": "Client not found",
			})
		}
		token := header[len("Bearer "):]
		_, ok := client.ValidToken(instance, permissions.RegistrationTokenAudience, token)
		if !ok {
			return c.JSON(http.StatusUnauthorized, echo.Map{
				"error": "invalid_token",
			})
		}
		c.Set("client", client)
		return next(c)
	}
}

func passphraseResetForm(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	return c.Render(http.StatusOK, "passphrase_reset.html", echo.Map{
		"Domain": instance.ContextualDomain(),
		"Locale": instance.Locale,
		"CSRF":   c.Get("csrf"),
	})
}

func passphraseReset(c echo.Context) error {
	i := middlewares.GetInstance(c)
	// TODO: check user informations to allow the reset of the passphrase since
	// this route is of course not protected by authentication/permission check.
	if err := i.RequestPassphraseReset(); err != nil && err != instance.ErrResetAlreadyRequested {
		return err
	}
	// Disconnect the user if it is logged in. The idea is that if the user
	// (maybe by accident) asks for a passphrase reset while logged in, we log
	// him out to be able to re-go through the process of logging back-in. It is
	// more a UX choice than a "security" one.
	session, ok := middlewares.GetSession(c)
	if ok {
		c.SetCookie(session.Delete(i))
	}
	return c.Render(http.StatusOK, "error.html", echo.Map{
		"Domain":     i.ContextualDomain(),
		"ErrorTitle": "Passphrase is reset Title",
		"Error":      "Passphrase is reset Body",
		"Button":     "Passphrase is reset Login Button",
		"ButtonLink": i.PageURL("/auth/login", nil),
	})
}

func passphraseRenewForm(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if middlewares.IsLoggedIn(c) {
		redirect := inst.DefaultRedirection().String()
		return c.Redirect(http.StatusSeeOther, redirect)
	}
	// Check that the token is actually defined and well encoded. The actual
	// token value checking is also done on the passphraseRenew handler.
	token, err := hex.DecodeString(c.QueryParam("token"))
	if err != nil || len(token) == 0 {
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Domain": inst.ContextualDomain(),
			"Error":  "Error Invalid reset token",
		})
	}
	if err = inst.CheckPassphraseRenewToken(token); err != nil {
		if err == instance.ErrMissingToken {
			return c.Render(http.StatusBadRequest, "error.html", echo.Map{
				"Domain": inst.ContextualDomain(),
				"Error":  "Error Invalid reset token",
			})
		}
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid_token",
		})
	}
	return c.Render(http.StatusOK, "passphrase_renew.html", echo.Map{
		"Domain":               inst.ContextualDomain(),
		"Locale":               inst.Locale,
		"PassphraseResetToken": hex.EncodeToString(token),
		"CSRF":                 c.Get("csrf"),
	})
}

func passphraseRenew(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if middlewares.IsLoggedIn(c) {
		redirect := inst.DefaultRedirection().String()
		return c.Redirect(http.StatusSeeOther, redirect)
	}
	pass := []byte(c.FormValue("passphrase"))
	token, err := hex.DecodeString(c.FormValue("passphrase_reset_token"))
	if err != nil {
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Domain": inst.ContextualDomain(),
			"Error":  "Error Invalid reset token",
		})
	}
	if err := inst.PassphraseRenew(pass, token); err != nil {
		if err == instance.ErrMissingToken {
			return c.Render(http.StatusBadRequest, "error.html", echo.Map{
				"Domain": inst.ContextualDomain(),
				"Error":  "Error Invalid reset token",
			})
		}
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid_token",
		})
	}
	return c.Redirect(http.StatusSeeOther, inst.PageURL("/auth/login", nil))
}

// Routes sets the routing for the status service
func Routes(router *echo.Group) {
	noCSRF := middleware.CSRFWithConfig(middleware.CSRFConfig{
		TokenLookup:    "form:csrf_token",
		CookieMaxAge:   3600, // 1 hour
		CookieHTTPOnly: true,
		CookieSecure:   !config.IsDevRelease(),
	})

	router.GET("/login", loginForm, noCSRF)
	router.POST("/login", login, noCSRF)

	router.DELETE("/login/others", logoutOthers)
	router.OPTIONS("/login/others", logoutPreflight)
	router.DELETE("/login", logout)
	router.OPTIONS("/login", logoutPreflight)

	router.GET("/passphrase_reset", passphraseResetForm, noCSRF)
	router.POST("/passphrase_reset", passphraseReset, noCSRF)
	router.GET("/passphrase_renew", passphraseRenewForm, noCSRF)
	router.POST("/passphrase_renew", passphraseRenew, noCSRF)

	router.POST("/register", registerClient, middlewares.AcceptJSON, middlewares.ContentTypeJSON)
	router.GET("/register/:client-id", readClient, middlewares.AcceptJSON, checkRegistrationToken)
	router.PUT("/register/:client-id", updateClient, middlewares.AcceptJSON, middlewares.ContentTypeJSON, checkRegistrationToken)
	router.DELETE("/register/:client-id", deleteClient, checkRegistrationToken)

	authorizeGroup := router.Group("/authorize", noCSRF)
	authorizeGroup.GET("", authorizeForm)
	authorizeGroup.POST("", authorize)
	authorizeGroup.GET("/sharing", authorizeSharingForm)
	authorizeGroup.POST("/sharing", authorizeSharing)
	authorizeGroup.GET("/app", authorizeAppForm)
	authorizeGroup.POST("/app", authorizeApp)

	router.POST("/access_token", accessToken)
}
