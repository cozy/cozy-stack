// Package auth provides register and login handlers
package auth

import (
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/cozy/cozy-stack/pkg/apps"
	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/sessions"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
)

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

func redirectSuccessLogin(c echo.Context, redirect string) error {
	instance := middlewares.GetInstance(c)

	session, err := sessions.New(instance)
	if err != nil {
		return err
	}
	cookie, err := session.ToCookie()
	if err != nil {
		return err
	}

	redirect = addCodeToRedirect(redirect, instance.Domain, session.ID())
	c.SetCookie(cookie)
	return c.Redirect(http.StatusSeeOther, redirect)
}

func registerPassphrase(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	registerToken, err := hex.DecodeString(c.FormValue("registerToken"))
	if err != nil {
		return jsonapi.NewError(http.StatusBadRequest, err)
	}

	passphrase := []byte(c.FormValue("passphrase"))
	if err := instance.RegisterPassphrase(passphrase, registerToken); err != nil {
		return jsonapi.BadRequest(err)
	}

	return redirectSuccessLogin(c, instance.SubDomain(apps.OnboardingSlug))
}

func updatePassphrase(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	newPassphrase := []byte(c.FormValue("new-passphrase"))
	currentPassphrase := []byte(c.FormValue("current-passphrase"))
	if err := instance.UpdatePassphrase(newPassphrase, currentPassphrase); err != nil {
		return jsonapi.BadRequest(err)
	}

	return redirectSuccessLogin(c, instance.SubDomain(apps.HomeSlug))
}

func loginForm(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	redirect, err := checkRedirectParam(c, instance.SubDomain(apps.HomeSlug))
	if err != nil {
		return err
	}

	session, err := sessions.GetSession(c, instance)
	if err == nil {
		redirect = addCodeToRedirect(redirect, instance.Domain, session.ID())
		return c.Redirect(http.StatusSeeOther, redirect)
	}

	return c.Render(http.StatusOK, "login.html", echo.Map{
		"InvalidPassphrase": false,
		"Redirect":          redirect,
	})
}

func login(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	redirect, err := checkRedirectParam(c, instance.SubDomain(apps.HomeSlug))
	if err != nil {
		return err
	}

	session, err := sessions.GetSession(c, instance)
	if err == nil {
		redirect = addCodeToRedirect(redirect, instance.Domain, session.ID())
		return c.Redirect(http.StatusSeeOther, redirect)
	}

	passphrase := []byte(c.FormValue("passphrase"))
	if err := instance.CheckPassphrase(passphrase); err == nil {
		return redirectSuccessLogin(c, redirect)
	}

	return c.Render(http.StatusUnauthorized, "login.html", echo.Map{
		"InvalidPassphrase": true,
		"Redirect":          redirect,
	})
}

func logout(c echo.Context) error {
	// TODO check that a valid CtxToken is given to protect against CSRF attacks
	instance := middlewares.GetInstance(c)

	session, err := sessions.GetSession(c, instance)
	if err == nil {
		c.SetCookie(session.Delete(instance))
	}

	return c.Redirect(http.StatusSeeOther, instance.PageURL("/auth/login", nil))
}

// checkRedirectParam returns the optional redirect query parameter. If not
// empty, we check that the redirect is a subdomain of the cozy-instance.
func checkRedirectParam(c echo.Context, defaultRedirect string) (string, error) {
	redirect := c.FormValue("redirect")
	if redirect == "" {
		redirect = defaultRedirect
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
		parts := strings.SplitN(u.Host, ".", 2)
		if len(parts) != 2 || parts[1] != instance.Domain || parts[0] == "" {
			return "", echo.NewHTTPError(http.StatusBadRequest,
				"bad url: should be subdomain")
		}
	}

	// To protect against stealing authorization code with redirection, the
	// fragment is always overriden. Most browsers keep URI fragments upon
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
			"Error": "The state parameter is mandatory",
		})
	}
	if params.clientID == "" {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "The client_id parameter is mandatory",
		})
	}
	if params.redirectURI == "" {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "The redirect_uri parameter is mandatory",
		})
	}
	if params.scope == "" {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "The scope parameter is mandatory",
		})
	}

	params.client = new(oauth.Client)
	if err := couchdb.GetDoc(params.instance, consts.OAuthClients, params.clientID, params.client); err != nil {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "The client must be registered",
		})
	}
	if !params.client.AcceptRedirectURI(params.redirectURI) {
		return true, c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "The redirect_uri parameter doesn't match the registered ones",
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
			"Error": "Invalid response type",
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
			"Error": "You must be authenticated",
		})
	}

	u, err := url.ParseRequestURI(params.redirectURI)
	if err != nil {
		return c.Render(http.StatusBadRequest, "error.html", echo.Map{
			"Error": "The redirect_uri parameter is invalid",
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

// Routes sets the routing for the status service
func Routes(router *echo.Group) {
	noCSRF := middleware.CSRFWithConfig(middleware.CSRFConfig{
		TokenLookup:    "form:csrf_token",
		CookieMaxAge:   3600, // 1 hour
		CookieHTTPOnly: true,
		CookieSecure:   !config.IsDevRelease(),
	})

	router.POST("/passphrase", registerPassphrase)
	router.PUT("/passphrase", updatePassphrase)

	router.GET("/login", loginForm)
	router.POST("/login", login)
	router.DELETE("/login", logout)

	router.POST("/register", registerClient, middlewares.AcceptJSON, middlewares.ContentTypeJSON)
	router.GET("/register/:client-id", readClient, middlewares.AcceptJSON, checkRegistrationToken)
	router.PUT("/register/:client-id", updateClient, middlewares.AcceptJSON, middlewares.ContentTypeJSON, checkRegistrationToken)
	router.DELETE("/register/:client-id", deleteClient, checkRegistrationToken)

	authorizeGroup := router.Group("/authorize", noCSRF)
	authorizeGroup.GET("", authorizeForm)
	authorizeGroup.POST("", authorize)

	router.POST("/access_token", accessToken)
}
