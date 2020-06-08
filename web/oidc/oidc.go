package oidc

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/session"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/statik"
	"github.com/labstack/echo/v4"
)

// Start is the route to start the OpenID Connect dance.
func Start(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	conf, err := getConfig(inst.ContextName)
	if err != nil {
		return renderError(c, nil, http.StatusNotFound, "Sorry, the context was not found.")
	}
	u, err := makeStartURL(inst.Domain, c.QueryParam("redirect"), conf)
	if err != nil {
		return renderError(c, nil, http.StatusNotFound, "Sorry, the server is not configured for OpenID Connect.")
	}
	return c.Redirect(http.StatusSeeOther, u)
}

// Redirect is the route after the Identity Provider has redirected the user to
// the stack. The redirection is made to a generic domain, like
// oauthcallback.cozy.tools and the association with an instance is made via a
// call to the UserInfo endpoint. It redirects to the cozy instance to login
// the user.
func Redirect(c echo.Context) error {
	code := c.QueryParam("code")
	stateID := c.QueryParam("state")
	state := getStorage().Find(stateID)
	if state == nil {
		return renderError(c, nil, http.StatusNotFound, "Sorry, the session has expired.")
	}

	domain := state.Instance
	if contextName, ok := FindLoginDomain(domain); ok {
		conf, err := getConfig(contextName)
		if err != nil || !conf.AllowOAuthToken {
			return renderError(c, nil, http.StatusBadRequest, "No OpenID Connect is configured.")
		}
		token := c.QueryParam("access_token")
		domain, err = getDomainFromUserInfo(conf, token)
		if err != nil {
			return renderError(c, nil, http.StatusNotFound, "Sorry, the cozy was not found.")
		}
	}
	inst, err := lifecycle.GetInstance(domain)
	if err != nil {
		return renderError(c, nil, http.StatusNotFound, "Sorry, the cozy was not found.")
	}

	u := url.Values{
		"code":  {code},
		"state": {stateID},
	}
	if state.Redirect != "" {
		u.Add("redirect", state.Redirect)
	}
	redirect := inst.PageURL("/oidc/login", u)
	return c.Redirect(http.StatusSeeOther, redirect)
}

// Login checks that the OpenID Connect has been successful and logs in the user.
func Login(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	conf, err := getConfig(inst.ContextName)
	if err != nil {
		return renderError(c, inst, http.StatusBadRequest, "No OpenID Connect is configured.")
	}

	var token string
	if conf.AllowOAuthToken {
		token = c.QueryParam("access_token")
	}
	if token == "" {
		code := c.QueryParam("code")
		token, err = getToken(conf, code)
		if err != nil {
			logger.WithNamespace("oidc").Errorf("Error on getToken: %s", err)
			return renderError(c, inst, http.StatusBadGateway, "Error from the identity provider.")
		}
	}

	if err := checkDomainFromUserInfo(conf, inst, token); err != nil {
		return renderError(c, inst, http.StatusBadRequest, err.Error())
	}

	if inst.HasAuthMode(instance.TwoFactorMail) {
		twoFactorToken, err := lifecycle.SendTwoFactorPasscode(inst)
		if err != nil {
			return err
		}
		v := url.Values{}
		v.Add("two_factor_token", string(twoFactorToken))
		v.Add("trusted_device_checkbox", "false")
		if redirect := c.QueryParam("redirect"); redirect != "" {
			v.Add("redirect", redirect)
		}
		// We can not cleanly check the trusted_device option for external
		// login. Therefore, we do not provide the checkbox
		return c.Redirect(http.StatusSeeOther, inst.PageURL("/auth/twofactor", v))
	}

	sessionID, err := auth.SetCookieForNewSession(c, false)
	if err != nil {
		return err
	}
	if err = session.StoreNewLoginEntry(inst, sessionID, "", c.Request(), true); err != nil {
		inst.Logger().Errorf("Could not store session history %q: %s", sessionID, err)
	}
	redirect := c.QueryParam("redirect")
	if redirect == "" {
		redirect = inst.DefaultRedirection().String()
	}
	return c.Redirect(http.StatusSeeOther, redirect)
}

// AccessToken delivers an access_token and a refresh_token if the client gives
// a valid token for OIDC.
func AccessToken(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	conf, err := getConfig(inst.ContextName)
	if err != nil || !conf.AllowOAuthToken {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "this endpoint is not enabled",
		})
	}

	var reqBody struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		Scope        string `json:"scope"`
		OIDCToken    string `json:"oidc_token"`
	}
	if err = c.Bind(&reqBody); err != nil {
		return err
	}

	// Check the token from the remote URL.
	if err := checkDomainFromUserInfo(conf, inst, reqBody.OIDCToken); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": err.Error(),
		})
	}

	// Load the OAuth client
	client, err := oauth.FindClient(inst, reqBody.ClientID)
	if err != nil {
		if couchErr, isCouchErr := couchdb.IsCouchError(err); isCouchErr && couchErr.StatusCode >= 500 {
			return err
		}
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "the client must be registered",
		})
	}
	if subtle.ConstantTimeCompare([]byte(reqBody.ClientSecret), []byte(client.ClientSecret)) == 0 {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid client_secret",
		})
	}

	// Prepare the scope
	out := auth.AccessTokenReponse{
		Type:  "bearer",
		Scope: reqBody.Scope,
	}
	if slug := oauth.GetLinkedAppSlug(client.SoftwareID); slug != "" {
		if err := auth.CheckLinkedAppInstalled(inst, slug); err != nil {
			return err
		}
		out.Scope = oauth.BuildLinkedAppScope(slug)
	}
	if out.Scope == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid scope",
		})
	}

	// Generate the access/refresh tokens
	accessToken, err := client.CreateJWT(inst, consts.AccessTokenAudience, out.Scope)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": "Can't generate access token",
		})
	}
	out.Access = accessToken
	refreshToken, err := client.CreateJWT(inst, consts.RefreshTokenAudience, out.Scope)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": "Can't generate refresh token",
		})
	}
	out.Refresh = refreshToken
	return c.JSON(http.StatusOK, out)
}

// Config is the config to log in a user with an OpenID Connect identity
// provider.
type Config struct {
	AllowOAuthToken     bool
	AllowCustomInstance bool
	ClientID            string
	ClientSecret        string
	Scope               string
	RedirectURI         string
	AuthorizeURL        string
	TokenURL            string
	UserInfoURL         string
	UserInfoField       string
	UserInfoPrefix      string
	UserInfoSuffix      string
}

func getConfig(context string) (*Config, error) {
	auth := config.GetConfig().Authentication
	delegated, ok := auth[context].(map[string]interface{})
	if !ok {
		return nil, errors.New("No authentication is configured for this context")
	}
	oidc, ok := delegated["oidc"].(map[string]interface{})
	if !ok {
		return nil, errors.New("No OIDC is configured for this context")
	}

	/* Mandatory fields */
	clientID, ok := oidc["client_id"].(string)
	if !ok {
		return nil, errors.New("The client_id is missing for this context")
	}
	clientSecret, ok := oidc["client_secret"].(string)
	if !ok {
		return nil, errors.New("The client_secret is missing for this context")
	}
	scope, ok := oidc["scope"].(string)
	if !ok {
		return nil, errors.New("The scope is missing for this context")
	}
	redirectURI, ok := oidc["redirect_uri"].(string)
	if !ok {
		return nil, errors.New("The redirect_uri is missing for this context")
	}
	authorizeURL, ok := oidc["authorize_url"].(string)
	if !ok {
		return nil, errors.New("The authorize_url is missing for this context")
	}
	tokenURL, ok := oidc["token_url"].(string)
	if !ok {
		return nil, errors.New("The token_url is missing for this context")
	}
	userInfoURL, ok := oidc["userinfo_url"].(string)
	if !ok {
		return nil, errors.New("The userinfo_url is missing for this context")
	}
	userInfoField, ok := oidc["userinfo_instance_field"].(string)
	if !ok {
		return nil, errors.New("The userinfo_instance_field is missing for this context")
	}

	/* Optional fields */
	allowOAuthToken, _ := oidc["allow_oauth_token"].(bool)
	allowCustomInstance, _ := oidc["allow_custom_instance"].(bool)
	userInfoPrefix, _ := oidc["userinfo_instance_prefix"].(string)
	userInfoSuffix, _ := oidc["userinfo_instance_suffix"].(string)

	config := &Config{
		AllowOAuthToken:     allowOAuthToken,
		AllowCustomInstance: allowCustomInstance,
		ClientID:            clientID,
		ClientSecret:        clientSecret,
		Scope:               scope,
		RedirectURI:         redirectURI,
		AuthorizeURL:        authorizeURL,
		TokenURL:            tokenURL,
		UserInfoURL:         userInfoURL,
		UserInfoField:       userInfoField,
		UserInfoPrefix:      userInfoPrefix,
		UserInfoSuffix:      userInfoSuffix,
	}
	return config, nil
}

func makeStartURL(domain, redirect string, conf *Config) (string, error) {
	u, err := url.Parse(conf.AuthorizeURL)
	if err != nil {
		return "", err
	}
	state := newStateHolder(domain, redirect)
	if err = getStorage().Add(state); err != nil {
		return "", err
	}
	vv := u.Query()
	vv.Add("response_type", "code")
	vv.Add("scope", conf.Scope)
	vv.Add("client_id", conf.ClientID)
	vv.Add("redirect_uri", conf.RedirectURI)
	vv.Add("state", state.id)
	vv.Add("nonce", state.Nonce)
	u.RawQuery = vv.Encode()
	return u.String(), nil
}

var oidcClient = &http.Client{
	Timeout: 15 * time.Second,
}

func getToken(conf *Config, code string) (string, error) {
	data := url.Values{
		"grant_type":   []string{"authorization_code"},
		"code":         []string{code},
		"redirect_uri": []string{conf.RedirectURI},
	}
	body := strings.NewReader(data.Encode())
	req, err := http.NewRequest("POST", conf.TokenURL, body)
	if err != nil {
		return "", err
	}
	auth := []byte(conf.ClientID + ":" + conf.ClientSecret)
	req.Header.Add("Authorization", "Basic "+base64.StdEncoding.EncodeToString(auth))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Accept", "application/json")

	res, err := oidcClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("OIDC service responded with %d", res.StatusCode)
	}
	resBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	var out struct {
		AccessToken string `json:"access_token"`
	}
	err = json.Unmarshal(resBody, &out)
	if err != nil {
		return "", err
	}
	return out.AccessToken, nil
}

func getDomainFromUserInfo(conf *Config, token string) (string, error) {
	if conf.AllowCustomInstance {
		return "", errors.New("invalid configuration")
	}
	params, err := getUserInfo(conf, token)
	if err != nil {
		return "", err
	}
	return extractDomain(conf, params)
}

func checkDomainFromUserInfo(conf *Config, inst *instance.Instance, token string) error {
	params, err := getUserInfo(conf, token)
	if err != nil {
		return err
	}

	if conf.AllowCustomInstance {
		sub, ok := params["sub"].(string)
		if !ok || sub == "" || sub != inst.OIDCID {
			logger.WithNamespace("oidc").Errorf("Invalid sub: %s != %s", sub, inst.OIDCID)
			return errors.New("the cozy was not found")
		}
		return nil
	}

	domain, err := extractDomain(conf, params)
	if err != nil {
		return err
	}
	if domain != inst.Domain {
		logger.WithNamespace("oidc").Errorf("Invalid domains: %s != %s", domain, inst.Domain)
		return errors.New("the cozy was not found")
	}
	return nil
}

func getUserInfo(conf *Config, token string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", conf.UserInfoURL, nil)
	if err != nil {
		return nil, errors.New("invalid configuration")
	}
	req.Header.Add("Authorization", "Bearer "+token)
	res, err := oidcClient.Do(req)
	if err != nil {
		logger.WithNamespace("oidc").Errorf("Error on getDomainFromUserInfo: %s", err)
		return nil, errors.New("error from the identity provider")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("OIDC service responded with %d", res.StatusCode)
	}

	var params map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&params)
	if err != nil {
		logger.WithNamespace("oidc").Errorf("Error on getDomainFromUserInfo: %s", err)
		return nil, errors.New("Invalid response from the identity provider")
	}
	return params, nil
}

func extractDomain(conf *Config, params map[string]interface{}) (string, error) {
	domain, ok := params[conf.UserInfoField].(string)
	if !ok {
		return "", errors.New("the cozy was not found")
	}
	domain = strings.Replace(domain, "-", "", -1) // We don't want - in cozy instance
	domain = strings.ToLower(domain)              // The domain is case insensitive
	domain = conf.UserInfoPrefix + domain + conf.UserInfoSuffix
	return domain, nil
}

func renderError(c echo.Context, inst *instance.Instance, code int, msg string) error {
	if inst == nil {
		inst = &instance.Instance{}
	}
	return c.Render(code, "error.html", echo.Map{
		"Title":       inst.TemplateTitle(),
		"CozyUI":      middlewares.CozyUI(inst),
		"ThemeCSS":    middlewares.ThemeCSS(inst),
		"Domain":      inst.ContextualDomain(),
		"ContextName": inst.ContextName,
		"Error":       msg,
		"Favicon":     middlewares.Favicon(inst),
	})
}

// Routes setup routing for OpenID Connect routes.
// Careful, the normal middlewares NeedInstance and LoadSession are not applied
// to this group in web/routing
func Routes(router *echo.Group) {
	router.GET("/start", Start, middlewares.NeedInstance, middlewares.CheckOnboardingNotFinished)
	router.GET("/redirect", Redirect)
	router.GET("/login", Login, middlewares.NeedInstance)
	router.POST("/access_token", AccessToken, middlewares.NeedInstance)
}

// LoginDomainHandler is the handler for the requests on the login domain. It
// shows a page with a login button (that can start the OIDC dance).
func LoginDomainHandler(c echo.Context, contextName string) error {
	r := c.Request()
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		rndr, err := statik.NewRenderer()
		if err != nil {
			return err
		}
		rndr.ServeHTTP(c.Response(), r)
		return nil
	}

	if r.Method != http.MethodPost {
		i := &instance.Instance{Locale: "fr", ContextName: contextName}
		title := i.Translate("Login Welcome")
		return c.Render(http.StatusOK, "oidc_login.html", echo.Map{
			"CozyUI":      middlewares.CozyUI(i),
			"ThemeCSS":    middlewares.ThemeCSS(i),
			"Favicon":     middlewares.Favicon(i),
			"Domain":      i.ContextualDomain(),
			"ContextName": i.ContextName,
			"Locale":      i.Locale,
			"Title":       title,
		})
	}

	conf, err := getConfig(contextName)
	if err != nil {
		return renderError(c, nil, http.StatusNotFound, "Sorry, the context was not found.")
	}
	u, err := makeStartURL(r.Host, "", conf)
	if err != nil {
		return renderError(c, nil, http.StatusNotFound, "Sorry, the server is not configured for OpenID Connect.")
	}
	return c.Redirect(http.StatusSeeOther, u)
}

// FindLoginDomain returns the context name for which the login domain matches
// the host.
func FindLoginDomain(host string) (string, bool) {
	for ctx, auth := range config.GetConfig().Authentication {
		delegated, ok := auth.(map[string]interface{})
		if !ok {
			continue
		}
		oidc, ok := delegated["oidc"].(map[string]interface{})
		if !ok {
			continue
		}
		domain, ok := oidc["login_domain"].(string)
		if ok && domain == host {
			return ctx, true
		}
	}
	return "", false
}
