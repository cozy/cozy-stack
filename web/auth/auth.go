// Package auth provides register and login handlers
package auth

import (
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/sessions"
	"github.com/cozy/cozy-stack/web/middlewares"
	webpermissions "github.com/cozy/cozy-stack/web/permissions"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
)

// CredentialsErrorKey is the key for translating the message showed to the
// user when he/she enters incorrect credentials
const CredentialsErrorKey = "Login Credentials error"

// Home is the handler for /
// It redirects to the login page is the user is not yet authentified
// Else, it redirects to its home application (or onboarding)
func Home(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	if session, err := sessions.GetSession(c, instance); err == nil {
		redirect := defaultRedirectDomain(instance).String()
		redirect = addCodeToRedirect(redirect, instance.Domain, session.ID())
		return c.Redirect(http.StatusSeeOther, redirect)
	}

	if len(instance.RegisterToken) > 0 {
		sub := instance.SubDomain(consts.OnboardingSlug)
		sub.RawQuery = c.Request().URL.RawQuery
		return c.Redirect(http.StatusSeeOther, sub.String())
	}

	return c.Redirect(http.StatusSeeOther, instance.PageURL("/auth/login", nil))
}

// With the nested subdomains structure, the cookie set on the main domain can
// also be used to authentify the user on the apps subdomain. But with the flat
// subdomains structure, a new cookie is needed. To transfer the session, we
// add a code parameter to the redirect URL that can be exchanged to the
// cookie. The code can be used only once, is valid only one minute, and is
// specific to the app (it can't be used by another app).
func addCodeToRedirect(redirect, domain, sessionID string) string {
	// TODO add rate-limiting on the number of session codes generated
	if config.GetConfig().Subdomains == config.FlatSubdomains {
		u, err := url.Parse(redirect)
		if err == nil && u.Host != domain {
			q := u.Query()
			q.Set("code", sessions.BuildCode(sessionID, u.Host).Value)
			u.RawQuery = q.Encode()
			return u.String()
		}
	}
	return redirect
}

// defaultRedirectDomain returns the default URL used for redirection after
// login actions.
func defaultRedirectDomain(in *instance.Instance) *url.URL {
	return in.SubDomain(consts.FilesSlug)
}

// SetCookieForNewSession creates a new session and sets the cookie on echo context
func SetCookieForNewSession(c echo.Context) (string, error) {
	instance := middlewares.GetInstance(c)

	session, err := sessions.New(instance)
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

func renderLoginForm(c echo.Context, i *instance.Instance, code int, redirect string) error {
	doc := &couchdb.JSONDoc{}
	err := couchdb.GetDoc(i, consts.Settings, consts.InstanceSettingsID, doc)
	if err != nil {
		return err
	}

	var credsErrors string
	if code == http.StatusUnauthorized {
		credsErrors = i.Translate(CredentialsErrorKey)
	}

	return c.Render(code, "login.html", echo.Map{
		"Locale":           i.Locale,
		"PublicName":       doc.M["public_name"],
		"CredentialsError": credsErrors,
		"Redirect":         redirect,
	})
}

func loginForm(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	redirect, err := checkRedirectParam(c, defaultRedirectDomain(instance))
	if err != nil {
		return err
	}

	session, err := sessions.GetSession(c, instance)
	if err == nil {
		redirect = addCodeToRedirect(redirect, instance.Domain, session.ID())
		return c.Redirect(http.StatusSeeOther, redirect)
	}

	return renderLoginForm(c, instance, http.StatusOK, redirect)
}

func login(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	wantsJSON := c.Request().Header.Get("Accept") == "application/json"

	redirect, err := checkRedirectParam(c, defaultRedirectDomain(instance))
	if err != nil {
		return err
	}

	var sessionID string
	session, err := sessions.GetSession(c, instance)
	if err == nil {
		sessionID = session.ID()
	} else {
		passphrase := []byte(c.FormValue("passphrase"))
		if err := instance.CheckPassphrase(passphrase); err == nil {
			if sessionID, err = SetCookieForNewSession(c); err != nil {
				return err
			}
		}
	}

	if sessionID != "" {
		redirect = addCodeToRedirect(redirect, instance.Domain, sessionID)
		if wantsJSON {
			return c.JSON(http.StatusOK, echo.Map{"redirect": redirect})
		}
		return c.Redirect(http.StatusSeeOther, redirect)
	}

	if wantsJSON {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": instance.Translate(CredentialsErrorKey),
		})
	}

	return renderLoginForm(c, instance, http.StatusUnauthorized, redirect)
}

func logout(c echo.Context) error {
	res := c.Response()
	origin := c.Request().Header.Get(echo.HeaderOrigin)
	res.Header().Set(echo.HeaderAccessControlAllowOrigin, origin)
	res.Header().Set(echo.HeaderAccessControlAllowCredentials, "true")

	instance := middlewares.GetInstance(c)
	if !webpermissions.AllowLogout(c) {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "The user can logout only from client-side apps",
		})
	}

	session, err := sessions.GetSession(c, instance)
	if err == nil {
		c.SetCookie(session.Delete(instance))
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
func checkRedirectParam(c echo.Context, defaultRedirect *url.URL) (string, error) {
	redirect := c.FormValue("redirect")
	if redirect == "" {
		redirect = defaultRedirect.String()
	}

	u, err := url.Parse(redirect)
	if err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest,
			"bad url: could not parse")
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return "", echo.NewHTTPError(http.StatusBadRequest,
			"bad url: bad scheme")
	}

	instance := middlewares.GetInstance(c)
	if u.Host != instance.Domain {
		instanceHost, appSlug, _ := middlewares.SplitHost(u.Host)
		if instanceHost != instance.Domain || appSlug == "" {
			return "", echo.NewHTTPError(http.StatusBadRequest,
				"bad url: should be subdomain")
		}
	}

	// To protect against stealing authorization code with redirection, the
	// fragment is always overridden. Most browsers keep URI fragments upon
	// redirects, to make sure to override them, we put an empty one.
	//
	// see: oauthsecurity.com/#provider-in-the-middle
	// see: 7.4.2 OAuth2 in Action
	u.Fragment = ""
	return u.String() + "#", nil
}

func registerClient(c echo.Context) error {
	// TODO add rate-limiting to prevent DOS attacks
	client := new(oauth.Client)
	if err := c.Bind(client); err != nil {
		return err
	}
	instance := middlewares.GetInstance(c)
	if err := client.Create(instance); err != nil {
		return c.JSON(err.Code, err)
	}
	return c.JSON(http.StatusCreated, client)
}

func readClient(c echo.Context) error {
	client := c.Get("client").(oauth.Client)
	client.TransformIDAndRev()
	return c.JSON(http.StatusOK, client)
}

func updateClient(c echo.Context) error {
	// TODO add rate-limiting to prevent DOS attacks
	client := new(oauth.Client)
	if err := c.Bind(client); err != nil {
		return err
	}
	oldClient := c.Get("client").(oauth.Client)
	instance := middlewares.GetInstance(c)
	if err := client.Update(instance, &oldClient); err != nil {
		return c.JSON(err.Code, err)
	}
	return c.JSON(http.StatusOK, client)
}

func deleteClient(c echo.Context) error {
	client := c.Get("client").(oauth.Client)
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
	client      *oauth.Client
}

func checkAuthorizeParams(c echo.Context, params *authorizeParams) (bool, error) {
	if params.state == "" {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "Error No state parameter",
		})
	}
	if params.clientID == "" {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "Error No client_id parameter",
		})
	}
	if params.redirectURI == "" {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "Error No redirect_uri parameter",
		})
	}
	if params.scope == "" {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "Error No scope parameter",
		})
	}

	params.client = new(oauth.Client)
	if err := couchdb.GetDoc(params.instance, consts.OAuthClients, params.clientID, params.client); err != nil {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "Error No registered client",
		})
	}
	if !params.client.AcceptRedirectURI(params.redirectURI) {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "Error Incorrect redirect_uri",
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
	}

	if c.QueryParam("response_type") != "code" {
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "Error Invalid response type",
		})
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

	permissions := strings.Split(params.scope, " ")
	params.client.ClientID = params.client.CouchID
	return c.Render(http.StatusOK, "authorize.html", echo.Map{
		"Locale":      instance.Locale,
		"Client":      params.client,
		"State":       params.state,
		"RedirectURI": params.redirectURI,
		"Scope":       params.scope,
		"Permissions": permissions,
		"CSRF":        c.Get("csrf"),
	})
}

func authorize(c echo.Context) error {
	params := authorizeParams{
		instance:    middlewares.GetInstance(c),
		state:       c.FormValue("state"),
		clientID:    c.FormValue("client_id"),
		redirectURI: c.FormValue("redirect_uri"),
		scope:       c.FormValue("scope"),
	}

	if !middlewares.IsLoggedIn(c) {
		return c.Render(http.StatusUnauthorized, "error.html", echo.Map{
			"Error": "Error Must be authenticated",
		})
	}

	u, err := url.ParseRequestURI(params.redirectURI)
	if err != nil {
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "Error Invalid redirect_uri",
		})
	}

	hasError, err := checkAuthorizeParams(c, &params)
	if hasError {
		return err
	}

	access, err := oauth.CreateAccessCode(params.instance, params.clientID, params.scope)
	if err != nil {
		return err
	}

	q := u.Query()
	q.Set("access_code", access.Code)
	q.Set("state", params.state)
	q.Set("client_id", params.clientID)
	u.RawQuery = q.Encode()
	u.Fragment = ""

	return c.Redirect(http.StatusFound, u.String()+"#")
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
			log.Errorf("[oauth] Failed to delete the access code: %s", err)
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

	return c.JSON(http.StatusOK, out)
}

func checkRegistrationToken(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		header := c.Request().Header.Get("Authorization")
		parts := strings.Split(header, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
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
		_, ok := client.ValidToken(instance, permissions.RegistrationTokenAudience, parts[1])
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
		"Locale": instance.Locale,
		"CSRF":   c.Get("csrf"),
	})
}

func passphraseReset(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	// TODO: check user informations to allow the reset of the passphrase since
	// this route is of course not protected by authentication/permission check.
	if err := instance.RequestPassphraseReset(); err != nil {
		return err
	}
	// Disconnect the user if it is logged in. The idea is that if the user
	// (maybe by accident) asks for a passphrase reset while logged in, we log
	// him out to be able to re-go through the process of logging back-in. It is
	// more a UX choice than a "security" one.
	if middlewares.IsLoggedIn(c) {
		session, err := sessions.GetSession(c, instance)
		if err == nil {
			c.SetCookie(session.Delete(instance))
		}
	}
	return c.Redirect(http.StatusSeeOther, instance.PageURL("/auth/login", nil))
}

func passphraseRenewForm(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	if middlewares.IsLoggedIn(c) {
		redirect := defaultRedirectDomain(instance).String()
		return c.Redirect(http.StatusSeeOther, redirect)
	}
	token := c.QueryParam("token")
	// Check that the token is actually defined and well encoded. The actual
	// token value checking is done on the passphraseRenew handler.
	if _, err := hex.DecodeString(token); err != nil || token == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid_token",
		})
	}
	return c.Render(http.StatusOK, "passphrase_renew.html", echo.Map{
		"Locale":               instance.Locale,
		"PassphraseResetToken": token,
		"CSRF":                 c.Get("csrf"),
	})
}

func passphraseRenew(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	if middlewares.IsLoggedIn(c) {
		redirect := defaultRedirectDomain(instance).String()
		return c.Redirect(http.StatusSeeOther, redirect)
	}
	pass := []byte(c.FormValue("passphrase"))
	token, err := hex.DecodeString(c.FormValue("passphrase_reset_token"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid_token",
		})
	}
	if err := instance.PassphraseRenew(pass, token); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid_token",
		})
	}
	return c.Redirect(http.StatusSeeOther, instance.PageURL("/auth/login", nil))
}

// Routes sets the routing for the status service
func Routes(router *echo.Group) {
	noCSRF := middleware.CSRFWithConfig(middleware.CSRFConfig{
		TokenLookup:    "form:csrf_token",
		CookieMaxAge:   3600, // 1 hour
		CookieHTTPOnly: true,
		CookieSecure:   !config.IsDevRelease(),
	})

	router.GET("/login", loginForm)
	router.POST("/login", login)
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

	router.POST("/access_token", accessToken)
}
