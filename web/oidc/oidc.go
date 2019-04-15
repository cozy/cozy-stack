package oidc

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/permissions"
	"github.com/cozy/cozy-stack/pkg/sessions"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

// Start is the route to start the OpenID Connect dance.
func Start(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	conf, err := getConfig(inst.ContextName)
	if err != nil {
		return renderError(c, nil, http.StatusNotFound, "Sorry, the context was not found.")
	}
	u, err := makeStartURL(inst, conf)
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
	inst, err := lifecycle.GetInstance(state.Instance)
	if err != nil {
		return renderError(c, nil, http.StatusNotFound, "Sorry, the cozy was not found.")
	}

	redirect := inst.PageURL("/oidc/login", url.Values{
		"code":  {code},
		"state": {stateID},
	})
	return c.Redirect(http.StatusSeeOther, redirect)
}

// Login checks that the OpenID Connect has been sucessful and logs in the user.
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

	domain, err := getDomainFromUserInfo(conf, token)
	if err != nil {
		logger.WithNamespace("oidc").Errorf("Error on getDomainFromUserInfo: %s", err)
		return renderError(c, inst, http.StatusBadGateway, "Error from the identity provider.")
	}
	if domain != inst.Domain {
		logger.WithNamespace("oidc").Errorf("Invalid domains: %s != %s", domain, inst.Domain)
		return renderError(c, inst, http.StatusBadRequest, "Sorry, the cozy was not found.")
	}

	sessionID, err := auth.SetCookieForNewSession(c, false)
	if err != nil {
		return err
	}
	if err = sessions.StoreNewLoginEntry(inst, sessionID, "", c.Request(), true); err != nil {
		inst.Logger().Errorf("Could not store session history %q: %s", sessionID, err)
	}
	redirect := inst.DefaultRedirection()
	redirect = auth.AddCodeToRedirect(redirect, inst.Domain, sessionID)
	return c.Redirect(http.StatusSeeOther, redirect.String())
}

// Config is the config to log in a user with an OpenID Connect identity
// provider.
type Config struct {
	AllowOAuthToken bool
	ClientID        string
	ClientSecret    string
	Scope           string
	RedirectURI     string
	AuthorizeURL    string
	TokenURL        string
	UserInfoURL     string
	UserInfoField   string
	UserInfoPrefix  string
	UserInfoSuffix  string
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
	userInfoPrefix, _ := oidc["userinfo_instance_prefix"].(string)
	userInfoSuffix, _ := oidc["userinfo_instance_suffix"].(string)

	config := &Config{
		AllowOAuthToken: allowOAuthToken,
		ClientID:        clientID,
		ClientSecret:    clientSecret,
		Scope:           scope,
		RedirectURI:     redirectURI,
		AuthorizeURL:    authorizeURL,
		TokenURL:        tokenURL,
		UserInfoURL:     userInfoURL,
		UserInfoField:   userInfoField,
		UserInfoPrefix:  userInfoPrefix,
		UserInfoSuffix:  userInfoSuffix,
	}
	return config, nil
}

func makeStartURL(inst *instance.Instance, conf *Config) (string, error) {
	u, err := url.Parse(conf.AuthorizeURL)
	if err != nil {
		return "", err
	}
	state := newStateHolder(inst.Domain)
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
	req, err := http.NewRequest("GET", conf.UserInfoURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("Authorization", "Bearer "+token)
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

	var out map[string]interface{}
	err = json.Unmarshal(resBody, &out)
	if err != nil {
		return "", err
	}
	if domain, ok := out[conf.UserInfoField].(string); ok {
		domain = strings.Replace(domain, "-", "", -1) // We don't want - in cozy instance
		domain = strings.ToLower(domain)              // The domain is case insensitive
		return conf.UserInfoPrefix + domain + conf.UserInfoSuffix, nil
	}
	return "", errors.New("No domain was found")
}

func renderError(c echo.Context, inst *instance.Instance, code int, msg string) error {
	if inst == nil {
		inst = &instance.Instance{}
	}
	return c.Render(code, "error.html", echo.Map{
		"CozyUI":      middlewares.CozyUI(inst),
		"ThemeCSS":    middlewares.ThemeCSS(inst),
		"Domain":      inst.ContextualDomain(),
		"ContextName": inst.ContextName,
		"Error":       msg,
	})
}

func accessToken(c echo.Context) error {
	var reqBody struct {
		Scope     string `json:"scope,omitempty"`
		OIDCToken string `json:"oidc_token"`
	}
	err := c.Bind(&reqBody)
	if err != nil {
		return err
	}
	inst := middlewares.GetInstance(c)

	conf, err := getConfig(inst.ContextName)
	if err != nil {
		return renderError(c, inst, http.StatusBadRequest, "No OpenID Connect is configured.")
	}

	// Check the token from the remote URL.
	domain, err := getDomainFromUserInfo(conf, reqBody.OIDCToken)
	if err != nil {
		return err
	}
	if domain != inst.Domain {
		logger.WithNamespace("oidc").Errorf("Invalid domains: %s != %s", domain, inst.Domain)
		return renderError(c, inst, http.StatusBadRequest, "Sorry, the cozy was not found.")
	}

	// Find or create the client
	newClient := &oauth.Client{
		ClientName:   "OIDC Client",
		SoftwareID:   "oidc_client",
		RedirectURIs: []string{inst.DefaultRedirection().String()},
	}

	client, err := oauth.FindClientBySoftwareID(inst, "oidc_client")
	if err != nil {
		errc := newClient.Create(inst)
		if errc != nil {
			return err
		}
		client, err = oauth.FindClientBySoftwareID(inst, "oidc_client")
	}

	// Set the scope
	scope := reqBody.Scope
	isLinkedApp := auth.IsLinkedApp(client.SoftwareID)

	if scope == "" && !isLinkedApp {
		return renderError(c, inst, http.StatusBadRequest, "Sorry, the scope is invalid")
	}

	if isLinkedApp {
		// Get the scope from the app
		slug := auth.GetLinkedAppSlug(client.SoftwareID)
		_, err := auth.GetLinkedApp(inst, slug)
		if err != nil {
			return err
		}
		scope = auth.BuildLinkedAppScope(slug)
	}

	// Generate its access/refresh tokens
	accessToken, err := client.CreateJWT(inst, permissions.AccessTokenAudience, scope)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": "Can't generate access token",
		})
	}
	refreshToken, err := client.CreateJWT(inst, permissions.RefreshTokenAudience, scope)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"error": "Can't generate refresh token",
		})
	}

	// Build the returned data
	type accessTokenReponse struct {
		Type     string `json:"token_type"`
		Scope    string `json:"scope"`
		Access   string `json:"access_token"`
		Refresh  string `json:"refresh_token,omitempty"`
		Instance string `json:"instance"`
	}
	out := accessTokenReponse{
		Type:     "bearer",
		Scope:    scope,
		Access:   accessToken,
		Refresh:  refreshToken,
		Instance: domain,
	}

	return c.JSON(http.StatusOK, out)
}

// Routes setup routing for OpenID Connect routes.
// Careful, the normal middlewares NeedInstance and LoadSession are not applied
// to this group in web/routing
func Routes(router *echo.Group) {
	router.GET("/start", Start, middlewares.NeedInstance)
	router.GET("/redirect", Redirect)
	router.GET("/login", Login, middlewares.NeedInstance)
	router.POST("/access_token", accessToken, middlewares.NeedInstance)
}
