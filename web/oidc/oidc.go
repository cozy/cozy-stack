package oidc

import (
	"crypto/rsa"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/model/session"
	build "github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/cozy-stack/web/statik"
	jwt "github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
)

// Start is the route to start the OpenID Connect dance.
func Start(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	conf, err := getConfig(inst.ContextName)
	if err != nil {
		inst.Logger().WithNamespace("oidc").Infof("Start error: %s", err)
		return renderError(c, nil, http.StatusNotFound, "Sorry, the context was not found.")
	}
	u, err := makeStartURL(inst.Domain, c.QueryParam("redirect"), c.QueryParam("confirm_state"), conf)
	if err != nil {
		return renderError(c, nil, http.StatusNotFound, "Sorry, the server is not configured for OpenID Connect.")
	}
	return c.Redirect(http.StatusSeeOther, u)
}

// Redirect is the route after the Identity Provider has redirected the user to
// the stack. The redirection is made to a generic domain, like
// oauthcallback.cozy.localhost and the association with an instance is made via a
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
	if state.Confirm != "" {
		u.Add("confirm_state", state.Confirm)
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
	redirect := c.QueryParam("redirect")
	confirm := c.QueryParam("confirm_state")
	idToken := c.QueryParam("id_token")

	err = limits.CheckRateLimit(inst, limits.AuthType)
	if limits.IsLimitReachedOrExceeded(err) {
		if err = auth.LoginRateExceeded(inst); err != nil {
			inst.Logger().WithNamespace("oidc").Warn(err.Error())
		}
		return renderError(c, nil, http.StatusNotFound, "Sorry, the session has expired.")
	}

	if idToken != "" && conf.IDTokenKeyURL != "" {
		if err := checkIDToken(conf, inst, idToken); err != nil {
			return renderError(c, inst, http.StatusBadRequest, err.Error())
		}
	} else {
		var token string
		if conf.AllowOAuthToken {
			token = c.QueryParam("access_token")
		}
		if token == "" {
			stateID := c.QueryParam("state")
			state := getStorage().Find(stateID)
			if state == nil {
				return renderError(c, nil, http.StatusNotFound, "Sorry, the session has expired.")
			}
			code := c.QueryParam("code")
			token, err = getToken(conf, code)
			if err != nil {
				logger.WithNamespace("oidc").Errorf("Error on getToken: %s", err)
				return renderError(c, inst, http.StatusBadGateway, "Error from the identity provider.")
			}
		}

		// Check 2FA if enabled, and if yes, render an HTML page to check if
		// the browser has a trusted device token in its local storage.
		if inst.HasAuthMode(instance.TwoFactorMail) {
			return c.Render(http.StatusOK, "oidc_twofactor.html", echo.Map{
				"Domain":      inst.ContextualDomain(),
				"AccessToken": token,
				"Redirect":    redirect,
				"Confirm":     confirm,
			})
		}

		if err := checkDomainFromUserInfo(conf, inst, token); err != nil {
			return renderError(c, inst, http.StatusBadRequest, err.Error())
		}
	}

	return createSessionAndRedirect(c, inst, redirect, confirm)
}

func TwoFactor(c echo.Context) error {
	accessToken := c.FormValue("access-token")
	redirect := c.FormValue("redirect")
	confirm := c.FormValue("confirm")
	trustedDeviceToken := []byte(c.FormValue("trusted-device-token"))

	inst := middlewares.GetInstance(c)
	conf, err := getConfig(inst.ContextName)
	if err != nil {
		return renderError(c, inst, http.StatusBadRequest, "No OpenID Connect is configured.")
	}
	if err := checkDomainFromUserInfo(conf, inst, accessToken); err != nil {
		return renderError(c, inst, http.StatusBadRequest, err.Error())
	}

	if inst.ValidateTwoFactorTrustedDeviceSecret(c.Request(), trustedDeviceToken) {
		return createSessionAndRedirect(c, inst, redirect, confirm)
	}

	twoFactorToken, err := lifecycle.SendTwoFactorPasscode(inst)
	if err != nil {
		return err
	}
	v := url.Values{}
	v.Add("two_factor_token", string(twoFactorToken))
	if redirect != "" {
		v.Add("redirect", redirect)
	}
	if confirm != "" {
		v.Add("confirm", "true")
		v.Add("state", confirm)
	}
	return c.Redirect(http.StatusSeeOther, inst.PageURL("/auth/twofactor", v))
}

func createSessionAndRedirect(c echo.Context, inst *instance.Instance, redirect, confirm string) error {
	// The OIDC danse has been made to confirm the identity of the user, not
	// for creating a new session.
	if confirm != "" {
		return auth.ConfirmSuccess(c, inst, confirm)
	}

	sessionID, err := auth.SetCookieForNewSession(c, session.NormalRun)
	if err != nil {
		return err
	}
	if err = session.StoreNewLoginEntry(inst, sessionID, "", c.Request(), "OIDC", true); err != nil {
		inst.Logger().Errorf("Could not store session history %q: %s", sessionID, err)
	}
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
		IDToken      string `json:"id_token"`
	}
	if err = c.Bind(&reqBody); err != nil {
		return err
	}

	// Check the token from the remote URL.
	if reqBody.IDToken != "" {
		err = checkIDToken(conf, inst, reqBody.IDToken)
	} else {
		err = checkDomainFromUserInfo(conf, inst, reqBody.OIDCToken)
	}
	if err != nil {
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

	// Remove the pending flag on the OAuth client (if needed)
	if client.Pending {
		client.Pending = false
		client.ClientID = ""
		_ = couchdb.UpdateDoc(inst, client)
		client.ClientID = client.CouchID
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
	IDTokenKeyURL       string
}

func getConfig(context string) (*Config, error) {
	oidc, ok := config.GetOIDC(context)
	if !ok {
		return nil, errors.New("No OIDC is configured for this context")
	}

	// Optional fields
	allowOAuthToken, _ := oidc["allow_oauth_token"].(bool)
	allowCustomInstance, _ := oidc["allow_custom_instance"].(bool)
	userInfoPrefix, _ := oidc["userinfo_instance_prefix"].(string)
	userInfoSuffix, _ := oidc["userinfo_instance_suffix"].(string)
	idTokenKeyURL, _ := oidc["id_token_jwk_url"].(string)

	// Mandatory fields
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
	if !ok && !allowCustomInstance {
		return nil, errors.New("The userinfo_instance_field is missing for this context")
	}

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
		IDTokenKeyURL:       idTokenKeyURL,
	}
	return config, nil
}

func makeStartURL(domain, redirect, confirm string, conf *Config) (string, error) {
	u, err := url.Parse(conf.AuthorizeURL)
	if err != nil {
		return "", err
	}
	state := newStateHolder(domain, redirect, confirm)
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
	req.Header.Add(echo.HeaderAuthorization, "Basic "+base64.StdEncoding.EncodeToString(auth))
	req.Header.Add(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Add(echo.HeaderAccept, echo.MIMEApplicationJSON)

	res, err := oidcClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		// Flush the body, so that the connecion can be reused by keep-alive
		_, _ = io.Copy(ioutil.Discard, res.Body)
		logger.WithNamespace("oidc").
			Infof("Invalid status code %d for %s", res.StatusCode, conf.TokenURL)
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
			inst.Logger().WithNamespace("oidc").Errorf("Invalid sub: %s != %s", sub, inst.OIDCID)
			return errors.New("Error The authentication has failed")
		}
		return nil
	}

	domain, err := extractDomain(conf, params)
	if err != nil {
		return err
	}
	if domain != inst.Domain {
		logger.WithNamespace("oidc").Errorf("Invalid domains: %s != %s", domain, inst.Domain)
		return errors.New("Error The authentication has failed")
	}
	return nil
}

func getUserInfo(conf *Config, token string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", conf.UserInfoURL, nil)
	if err != nil {
		return nil, errors.New("invalid configuration")
	}
	req.Header.Add(echo.HeaderAuthorization, "Bearer "+token)
	res, err := oidcClient.Do(req)
	if err != nil {
		logger.WithNamespace("oidc").Errorf("Error on getDomainFromUserInfo: %s", err)
		return nil, errors.New("error from the identity provider")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		// Flush the body, so that the connecion can be reused by keep-alive
		_, _ = io.Copy(ioutil.Discard, res.Body)
		logger.WithNamespace("oidc").
			Infof("Invalid status code %d for %s", res.StatusCode, conf.UserInfoURL)
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
		return "", errors.New("Error The authentication has failed")
	}
	domain = strings.Replace(domain, "-", "", -1) // We don't want - in cozy instance
	domain = strings.ToLower(domain)              // The domain is case insensitive
	domain = conf.UserInfoPrefix + domain + conf.UserInfoSuffix
	return domain, nil
}

func checkIDToken(conf *Config, inst *instance.Instance, idToken string) error {
	keys, err := GetIDTokenKeys(conf.IDTokenKeyURL)
	if err != nil {
		return err
	}

	token, err := jwt.Parse(idToken, func(token *jwt.Token) (interface{}, error) {
		return ChooseKeyForIDToken(keys, token)
	})
	if err != nil {
		logger.WithNamespace("oidc").Errorf("Error on jwt.Parse: %s", err)
		return errors.New("invalid token")
	}
	if !token.Valid {
		logger.WithNamespace("oidc").Errorf("Invalid token: %#v", token)
		return errors.New("invalid token")
	}

	claims := token.Claims.(jwt.MapClaims)
	if claims["sub"] == "" || claims["sub"] != inst.OIDCID {
		inst.Logger().WithNamespace("oidc").Errorf("Invalid sub: %s != %s", claims["sub"], inst.OIDCID)
		return errors.New("Error The authentication has failed")
	}

	return nil
}

type jwKey struct {
	Alg  string `json:"alg"`
	Type string `json:"kty"`
	ID   string `json:"kid"`
	Use  string `json:"use"`
	E    string `json:"e"`
	N    string `json:"n"`
}

const cacheTTL = 24 * time.Hour

var keysClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		DisableKeepAlives: true,
	},
}

// GetIDTokenKeys returns the keys that can be used to verify that an OIDC
// id_token is valid.
func GetIDTokenKeys(keyURL string) ([]*jwKey, error) {
	cache := config.GetConfig().CacheStorage
	cacheKey := "oidc-jwk:" + keyURL

	data, ok := cache.Get(cacheKey)
	if !ok {
		var err error
		data, err = getKeysFromHTTP(keyURL)
		if err != nil {
			return nil, err
		}
	}

	var keys struct {
		Keys []*jwKey `json:"keys"`
	}
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, err
	}
	if !ok {
		cache.Set(cacheKey, data, cacheTTL)
	}
	return keys.Keys, nil
}

func getKeysFromHTTP(keyURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, keyURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("User-Agent", "cozy-stack "+build.Version+" ("+runtime.Version()+")")
	res, err := keysClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		logger.WithNamespace("oidc").Warnf("getKeys cannot fetch jwk: %d", res.StatusCode)
		return nil, errors.New("cannot fetch jwk")
	}
	return ioutil.ReadAll(res.Body)
}

// ChooseKeyForIDToken can be used to check an id_token as a JWT.
func ChooseKeyForIDToken(keys []*jwKey, token *jwt.Token) (interface{}, error) {
	if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
		return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
	}

	var key *jwKey
	for _, k := range keys {
		if k.Use != "sig" || k.Type != "RSA" {
			continue
		}
		if k.ID == token.Header["kid"] {
			return loadKey(k)
		}
		key = k
	}
	if key == nil {
		return nil, errors.New("Key not found")
	}
	return loadKey(key)
}

func loadKey(raw *jwKey) (interface{}, error) {
	var n, e big.Int
	nn, err := base64.RawURLEncoding.DecodeString(raw.N)
	if err != nil {
		return nil, err
	}
	n.SetBytes(nn)
	ee, err := base64.RawURLEncoding.DecodeString(raw.E)
	if err != nil {
		return nil, err
	}
	e.SetBytes(ee)

	var key rsa.PublicKey
	key.N = &n
	key.E = int(e.Int64())
	return &key, nil
}

func renderError(c echo.Context, inst *instance.Instance, code int, msg string) error {
	if inst == nil {
		inst = &instance.Instance{
			Domain:      c.Request().Host,
			ContextName: config.DefaultInstanceContext,
			Locale:      consts.DefaultLocale,
		}
	}
	return c.Render(code, "error.html", echo.Map{
		"Domain":       inst.ContextualDomain(),
		"ContextName":  inst.ContextName,
		"Locale":       inst.Locale,
		"Title":        inst.TemplateTitle(),
		"Favicon":      middlewares.Favicon(inst),
		"Illustration": "/images/generic-error.svg",
		"Error":        msg,
		"SupportEmail": inst.SupportEmailAddress(),
	})
}

// Routes setup routing for OpenID Connect routes.
// Careful, the normal middlewares NeedInstance and LoadSession are not applied
// to this group in web/routing
func Routes(router *echo.Group) {
	router.GET("/start", Start, middlewares.NeedInstance, middlewares.CheckOnboardingNotFinished)
	router.GET("/redirect", Redirect)
	router.GET("/login", Login, middlewares.NeedInstance)
	router.POST("/twofactor", TwoFactor, middlewares.NeedInstance)
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
			"Domain":      i.ContextualDomain(),
			"ContextName": i.ContextName,
			"Locale":      i.Locale,
			"Title":       title,
			"Favicon":     middlewares.Favicon(i),
		})
	}

	conf, err := getConfig(contextName)
	if err != nil {
		return renderError(c, nil, http.StatusNotFound, "Sorry, the context was not found.")
	}
	u, err := makeStartURL(r.Host, "", "", conf)
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
