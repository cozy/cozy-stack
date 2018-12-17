package settings

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

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
		// Generate the deeplink
		deeplink := fmt.Sprintf("cozy%s://%s", client.OnboardingApp, i.Domain)

		// Redirection
		queryParams := url.Values{
			"client_id":     {client.CouchID},
			"redirect_uri":  {deeplink},
			"state":         {client.OnboardingState},
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
