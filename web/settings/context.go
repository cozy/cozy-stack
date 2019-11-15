package settings

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/model/oauth"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

type apiContext struct {
	doc map[string]interface{}
}

func (c *apiContext) ID() string                             { return consts.ContextSettingsID }
func (c *apiContext) Rev() string                            { return "" }
func (c *apiContext) DocType() string                        { return consts.Settings }
func (c *apiContext) Fetch(field string) []string            { return nil }
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

func onboarded(c echo.Context) error {
	i := middlewares.GetInstance(c)
	if !middlewares.IsLoggedIn(c) {
		return c.Redirect(http.StatusSeeOther, i.PageURL("/auth/login", nil))
	}
	return finishOnboarding(c, true)
}

func finishOnboarding(c echo.Context, acceptHTML bool) error {
	i := middlewares.GetInstance(c)
	if !i.OnboardingFinished {
		t := true
		err := lifecycle.Patch(i, &lifecycle.Options{OnboardingFinished: &t})
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
		// If the redirectURI scheme is not starting with cozy<app>://, it means
		// that we probably are on a recent mobile, handling universal/android
		// links. We won't provide a fallbackURI because the redirectURI should
		// be enough to handle the redirection on the mobile-side
		var fallbackURI string
		if strings.HasPrefix(r.Scheme, "cozy") {
			appSlug := strings.TrimPrefix(client.SoftwareID, "registry://")
			fallbackURI = i.SubDomain(appSlug).String()
		}
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
	if acceptHTML {
		return c.Redirect(http.StatusSeeOther, redirect)
	}
	return c.JSON(http.StatusOK, echo.Map{"redirect": redirect})
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

	doc := &apiContext{normalize(ctx)}
	// Any request with a token can ask for the context (no permissions are required)
	if _, err = middlewares.GetPermission(c); err != nil {
		return echo.NewHTTPError(http.StatusForbidden)
	}

	return jsonapi.Data(c, http.StatusOK, doc, nil)
}

func normalize(input map[string]interface{}) map[string]interface{} {
	normalized := make(map[string]interface{}, len(input))
	for k, v := range input {
		switch v := v.(type) {
		case map[interface{}]interface{}:
			normalized[k] = doNormalizeMap(v)
		case []interface{}:
			normalized[k] = doNormalizeSlice(v)
		default:
			normalized[k] = v
		}
	}
	return normalized
}

func doNormalizeMap(input map[interface{}]interface{}) map[string]interface{} {
	normalized := make(map[string]interface{}, len(input))
	for k, v := range input {
		key := fmt.Sprintf("%v", k)
		switch v := v.(type) {
		case map[interface{}]interface{}:
			normalized[key] = doNormalizeMap(v)
		case []interface{}:
			normalized[key] = doNormalizeSlice(v)
		default:
			normalized[key] = v
		}
	}
	return normalized
}

func doNormalizeSlice(input []interface{}) []interface{} {
	normalized := make([]interface{}, len(input))
	for i, v := range input {
		switch v := v.(type) {
		case map[interface{}]interface{}:
			normalized[i] = doNormalizeMap(v)
		case []interface{}:
			normalized[i] = doNormalizeSlice(v)
		default:
			normalized[i] = v
		}
	}
	return normalized
}
