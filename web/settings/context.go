package settings

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/web/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

type apiContext struct {
	doc map[string]interface{}
}

func (c *apiContext) ID() string                             { return consts.ContextSettingsID }
func (c *apiContext) Rev() string                            { return "" }
func (c *apiContext) DocType() string                        { return consts.Settings }
func (c *apiContext) Clone() couchdb.Doc                     { return c }
func (c *apiContext) SetID(id string)                        {}
func (c *apiContext) SetRev(rev string)                      {}
func (c *apiContext) Relationships() jsonapi.RelationshipMap { return nil }
func (c *apiContext) Included() []jsonapi.Object             { return nil }
func (c *apiContext) Links() *jsonapi.LinksList {
	return &jsonapi.LinksList{Self: "/settings/context"}
}
func (c *apiContext) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.doc)
}
func (c *apiContext) Match(field, expected string) bool {
	return false
}

func onboarded(c echo.Context) error {
	i := middlewares.GetInstance(c)
	if !middlewares.IsLoggedIn(c) {
		return c.Redirect(http.StatusSeeOther, i.PageURL("/auth/login", nil))
	}
	return finishOnboarding(c)
}

func finishOnboarding(c echo.Context) error {
	i := middlewares.GetInstance(c)
	if !i.OnboardingFinished {
		t := true
		err := instance.Patch(i, &instance.Options{OnboardingFinished: &t})
		if err != nil {
			return err
		}
	}
	redirect := i.OnboardedRedirection().String()

	// Retreiving client
	// If there is no onboarding client, we keep going
	client, err := oauth.FindOnboardingClient(i)

	// Redirect to permissions screen if we are in a mobile onboarding
	if err == nil && client.OnboardingSecret != "" {
		redirectURI := ""
		if len(client.RedirectURIs) > 0 {
			redirectURI = client.RedirectURIs[0]
		}

		// Create and adding a fallbackURI in case of no-supporting custom
		// protocol cozy<app>://
		// Basically, it parses the app slug and computes the web app url
		// Example: cozydrive:// => http://drive.alice.cozy.tools:8080/
		r, err := url.Parse(redirectURI)
		if err != nil {
			return err
		}
		appSlug := strings.TrimLeft(r.Scheme, "cozy")
		fallbackURI := i.SubDomain(appSlug).String()

		// Redirection
		queryParams := url.Values{
			"client_id":     {client.CouchID},
			"redirect_uri":  {redirectURI},
			"state":         {client.OnboardingState},
			"fallback_uri":  {fallbackURI},
			"response_type": {"code"},
			"scope":         {client.OnboardingPermissions},
		}
		redirect = i.PageURL("/auth/authorize", queryParams)

	}
	return c.Redirect(http.StatusSeeOther, redirect)
}

func context(c echo.Context) error {
	i := middlewares.GetInstance(c)
	ctx, err := i.SettingsContext()
	if err == instance.ErrContextNotFound {
		return jsonapi.NotFound(err)
	}
	if err != nil {
		return err
	}

	doc := &apiContext{ctx}
	if _, err = middlewares.GetPermission(c); err != nil {
		return echo.NewHTTPError(http.StatusForbidden)
	}

	return jsonapi.Data(c, http.StatusOK, doc, nil)
}
