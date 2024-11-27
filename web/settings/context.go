package settings

import (
	"encoding/json"
	"net/http"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/jsonapi"
	"github.com/cozy/cozy-stack/web/auth"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
	"github.com/mssola/user_agent"
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

func (h *HTTPHandler) onboarded(c echo.Context) error {
	i := middlewares.GetInstance(c)
	if !middlewares.IsLoggedIn(c) {
		return c.Redirect(http.StatusSeeOther, i.PageURL("/auth/login", nil))
	}
	return finishOnboarding(c, "", true)
}

func finishOnboarding(c echo.Context, redirection string, acceptHTML bool) error {
	i := middlewares.GetInstance(c)
	if !i.OnboardingFinished {
		t := true
		err := lifecycle.Patch(i, &lifecycle.Options{OnboardingFinished: &t})
		if err != nil {
			return err
		}
	}
	redirect := i.OnboardedRedirection().String()
	if redirection != "" {
		if u, err := auth.AppRedirection(i, redirection); err == nil {
			redirect = u.String()
		}
	}

	rawUserAgent := c.Request().UserAgent()
	ua := user_agent.New(rawUserAgent)
	if ua.Mobile() {
		redirect = i.PageURL("/settings/install_flagship_app", nil)
	}

	if acceptHTML {
		return c.Redirect(http.StatusSeeOther, redirect)
	}
	return c.JSON(http.StatusOK, echo.Map{"redirect": redirect})
}

func (h *HTTPHandler) context(c echo.Context) error {
	// Any request with a token can ask for the context (no permissions are required)
	if _, err := middlewares.GetPermission(c); err != nil {
		return echo.NewHTTPError(http.StatusForbidden)
	}

	i := middlewares.GetInstance(c)
	context := &apiContext{i.GetContextWithSponsorships()}

	managerURL, err := i.ManagerURL(instance.ManagerBaseURL)
	if err != nil {
		return err
	}
	if managerURL != "" {
		// XXX: The manager URL used to be stored in the config in
		// `context.<context_name>.manager_url`. It's now stored in
		// `clouderies.<context_name>.api.url` and can be retrieved via a call
		// to `instance.ManagerURL()`.
		//
		// However, some external apps and clients (e.g. `cozy-client`) still
		// expect to find the `manager_url` attribute in the context document
		// so we add it back for backwards compatibility.
		context.doc["manager_url"] = managerURL
	}

	return jsonapi.Data(c, http.StatusOK, context, nil)
}
