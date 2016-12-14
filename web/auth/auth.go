// Package auth provides register and login handlers
package auth

import (
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"

	"github.com/cozy/cozy-stack/apps"
	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/crypto"
	"github.com/cozy/cozy-stack/instance"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/labstack/gommon/log"
)

func redirectSuccessLogin(c echo.Context, redirect string) error {
	instance := middlewares.GetInstance(c)

	session, err := NewSession(instance)
	if err != nil {
		return err
	}

	cookie, err := session.ToCookie()
	if err != nil {
		return err
	}

	c.SetCookie(cookie)
	return c.Redirect(http.StatusSeeOther, redirect)
}

func register(c echo.Context) error {
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

func loginForm(c echo.Context) error {
	instance := middlewares.GetInstance(c)

	redirect, err := checkRedirectParam(c, instance.SubDomain(apps.HomeSlug))
	if err != nil {
		return err
	}

	if IsLoggedIn(c) {
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

	if IsLoggedIn(c) {
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

	session, err := GetSession(c)
	if err == nil {
		c.SetCookie(session.Delete(instance))
	}

	return c.Redirect(http.StatusSeeOther, instance.PageURL("/auth/login"))
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
	parts := strings.SplitN(u.Host, ".", 2)
	if len(parts) != 2 || parts[1] != instance.Domain || parts[0] == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest,
			"bad url: should be subdomain")
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
	if c.Request().Header.Get("Content-Type") != "application/json" {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "bad_content_type",
		})
	}
	client := new(Client)
	if err := c.Bind(client); err != nil {
		return err
	}
	instance := middlewares.GetInstance(c)
	if err := client.Create(instance); err != nil {
		return c.JSON(err.Code, err)
	}
	return c.JSON(http.StatusCreated, client)
}

type authorizeParams struct {
	instance    *instance.Instance
	state       string
	clientID    string
	redirectURI string
	scope       string
	client      *Client
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

	params.client = new(Client)
	if err := couchdb.GetDoc(params.instance, ClientDocType, params.clientID, params.client); err != nil {
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
	params := authorizeParams{
		instance:    middlewares.GetInstance(c),
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

	if !IsLoggedIn(c) {
		redirect := url.Values{
			"redirect": {c.Request().URL.String()},
		}
		u := url.URL{
			Scheme:   "https",
			Host:     params.instance.Domain,
			Path:     "/auth/login",
			RawQuery: redirect.Encode(),
		}
		return c.Redirect(http.StatusSeeOther, u.String())
	}

	// TODO Trust On First Use

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

	if !IsLoggedIn(c) {
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

	access, err := CreateAccessCode(params.instance, params.clientID, params.scope)
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

func accessToken(c echo.Context) error {
	if c.FormValue("grant_type") != "authorization_code" {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "only the authorization_code grant type is available",
		})
	}
	code := c.FormValue("code")
	clientID := c.FormValue("client_id")
	clientSecret := c.FormValue("client_secret")

	if code == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "the code parameter is mandatory",
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

	instance := middlewares.GetInstance(c)
	client := &Client{}
	if err := couchdb.GetDoc(instance, ClientDocType, clientID, client); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "the client must be registered",
		})
	}

	// TODO check that clientSecret is valid
	if clientSecret == "xxx" {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid client_secret",
		})
	}

	accessCode := &AccessCode{}
	if err := couchdb.GetDoc(instance, AccessCodeDocType, code, accessCode); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid code",
		})
	}
	scope := accessCode.Scope

	// TODO move code to a method
	accessToken, err := crypto.NewJWT(instance.OAuthSecret, jwt.StandardClaims{
		Audience: "access",
		Issuer:   instance.Domain,
		IssuedAt: crypto.Timestamp(),
		Subject:  client.ClientID, // TODO add a test about it
	})
	if err != nil {
		log.Errorf("[oauth] Failed to create the client access token: %s", err)
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": "Can't generate access token",
		})
	}

	refreshToken, err := crypto.NewJWT(instance.OAuthSecret, jwt.StandardClaims{
		Audience: "refresh",
		Issuer:   instance.Domain,
		IssuedAt: crypto.Timestamp(),
		Subject:  client.ClientID, // TODO add a test about it
	})
	if err != nil {
		log.Errorf("[oauth] Failed to create the client refresh token: %s", err)
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": "Can't generate refresh token",
		})
	}

	// Delete the access code, it can be used only once
	couchdb.DeleteDoc(instance, accessCode)

	// TODO add tests

	return c.JSON(http.StatusOK, echo.Map{
		"token_type":    "bearer",
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"scope":         scope,
	})
}

// IsLoggedIn returns true if the context has a valid session cookie.
func IsLoggedIn(c echo.Context) bool {
	_, err := GetSession(c)
	return err == nil
}

// Routes sets the routing for the status service
func Routes(router *echo.Group) {
	noCSRF := middleware.CSRFWithConfig(middleware.CSRFConfig{
		TokenLookup:    "form:csrf_token",
		CookieMaxAge:   3600, // 1 hour
		CookieHTTPOnly: true,
		CookieSecure:   true,
	})

	router.POST("/register", register)

	router.GET("/auth/login", loginForm)
	router.POST("/auth/login", login)
	router.DELETE("/auth/login", logout)

	router.POST("/auth/register", registerClient)

	authorizeGroup := router.Group("/auth/authorize", noCSRF)
	authorizeGroup.GET("", authorizeForm)
	authorizeGroup.POST("", authorize)

	router.POST("/auth/access_token", accessToken)
}
