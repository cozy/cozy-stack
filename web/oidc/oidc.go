package oidc

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

// Start is the route to start the OpenID Connect dance.
func Start(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	conf, err := getConfig(inst.ContextName)
	if err != nil {
		redirect := inst.DefaultRedirection()
		return c.Redirect(http.StatusSeeOther, redirect.String())
	}
	u, err := makeStartURL(inst, conf)
	if err != nil {
		redirect := inst.DefaultRedirection()
		return c.Redirect(http.StatusSeeOther, redirect.String())
	}
	return c.Redirect(http.StatusSeeOther, u)
}

// Redirect is the route after the Identity Provider has redirected the user to
// the stack. The redirection is made to a generic domain, like
// oauthcallback.cozy.tools and the association with an instance is made via a
// call to the UserInfo endpoint.
func Redirect(c echo.Context) error {
	return nil
}

// Config is the config to log in a user with an OpenID Connect identity
// provider.
type Config struct {
	ClientID      string
	ClientSecret  string
	Scope         string
	RedirectURI   string
	AuthorizeURL  string
	TokenURL      string
	UserInfoURL   string
	UserInfoField string
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

	config := &Config{
		ClientID:      clientID,
		ClientSecret:  clientSecret,
		Scope:         scope,
		RedirectURI:   redirectURI,
		AuthorizeURL:  authorizeURL,
		TokenURL:      tokenURL,
		UserInfoURL:   userInfoURL,
		UserInfoField: userInfoField,
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

// Routes setup routing for OpenID Connect routes.
// Careful, the normal middlewares NeedInstance and LoadSession are not applied
// to this group in web/routing
func Routes(router *echo.Group) {
	router.GET("/start", Start, middlewares.NeedInstance)
	router.GET("/redirect", Redirect)
}
