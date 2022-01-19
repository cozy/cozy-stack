package auth

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/limits"
	"github.com/cozy/cozy-stack/pkg/realtime"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/labstack/echo/v4"
)

func confirmForm(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	redirect := c.QueryParam("redirect")
	state := c.QueryParam("state")
	if state == "" {
		return renderError(c, http.StatusBadRequest, "Error No state parameter")
	}
	if !inst.IsPasswordAuthenticationEnabled() {
		q := url.Values{"redirect": {redirect}, "confirm_state": {state}}
		return c.Redirect(http.StatusSeeOther, inst.PageURL("/oidc/start", q))
	}

	iterations := 0
	if settings, err := settings.Get(inst); err == nil {
		iterations = settings.PassphraseKdfIterations
	}
	return c.Render(http.StatusOK, "confirm_auth.html", echo.Map{
		"TemplateTitle":  inst.TemplateTitle(),
		"Domain":         inst.ContextualDomain(),
		"ContextName":    inst.ContextName,
		"Locale":         inst.Locale,
		"Iterations":     iterations,
		"Salt":           string(inst.PassphraseSalt()),
		"CSRF":           c.Get("csrf"),
		"Favicon":        middlewares.Favicon(inst),
		"BottomNavBar":   middlewares.BottomNavigationBar(c),
		"CryptoPolyfill": middlewares.CryptoPolyfill(c),
		"State":          state,
		"Redirect":       redirect,
	})
}

func confirmAuth(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if !inst.IsPasswordAuthenticationEnabled() {
		return c.NoContent(http.StatusBadRequest)
	}
	state := c.FormValue("state")

	// Check passphrase
	passphrase := []byte(c.FormValue("passphrase"))
	if lifecycle.CheckPassphrase(inst, passphrase) != nil {
		errorMessage := inst.Translate(CredentialsErrorKey)
		err := limits.CheckRateLimit(inst, limits.AuthType)
		if limits.IsLimitReachedOrExceeded(err) {
			if err = LoginRateExceeded(inst); err != nil {
				inst.Logger().WithNamespace("auth").Warn(err.Error())
			}
		}
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": errorMessage,
		})
	}

	if inst.HasAuthMode(instance.TwoFactorMail) && !isTrustedDevice(c, inst) {
		twoFactorToken, err := lifecycle.SendTwoFactorPasscode(inst)
		if err != nil {
			return err
		}
		v := url.Values{}
		v.Add("two_factor_token", string(twoFactorToken))
		v.Add("state", state)
		v.Add("redirect", c.FormValue("redirect"))
		v.Add("confirm", "true")
		v.Add("trusted_device_checkbox", "false")

		return c.JSON(http.StatusOK, echo.Map{
			"redirect": inst.PageURL("/auth/twofactor", v),
		})
	}

	return ConfirmSuccess(c, inst, state)
}

// ConfirmSuccess can be used to send a response after a successful identity
// confirmation.
func ConfirmSuccess(c echo.Context, inst *instance.Instance, state string) error {
	doc := couchdb.JSONDoc{
		Type: consts.AuthConfirmations,
		M: map[string]interface{}{
			"_id": state,
		},
	}
	realtime.GetHub().Publish(inst, realtime.EventCreate, &doc, nil)

	redirect, err := checkRedirectToManager(c)
	if err != nil {
		redirect, err = checkRedirectParam(c, inst.DefaultRedirection())
	}
	if err != nil {
		return err
	}
	code, err := GetStore().AddCode(inst)
	if err != nil {
		inst.Logger().Warnf("Cannot add confirm code: %s", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	q := redirect.Query()
	q.Set("code", code)
	q.Set("state", state)
	redirect.RawQuery = q.Encode()

	if wantsJSON(c) {
		return c.JSON(http.StatusOK, echo.Map{
			"redirect": redirect.String(),
		})
	}
	return c.Redirect(http.StatusSeeOther, redirect.String())
}

func checkRedirectToManager(c echo.Context) (*url.URL, error) {
	inst := middlewares.GetInstance(c)

	config, ok := inst.SettingsContext()
	if !ok {
		return nil, errors.New("no manager_url")
	}
	managerURL, ok := config["manager_url"].(string)
	if !ok {
		return nil, errors.New("no manager_url")
	}
	manager, err := url.Parse(managerURL)
	if err != nil {
		return nil, errors.New("invalid manager_url")
	}

	redirect := c.FormValue("redirect")
	u, err := url.Parse(redirect)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "bad url: could not parse")
	}

	if u.Scheme != manager.Scheme || u.Host != manager.Host {
		return nil, errors.New("not a redirection to the manager")
	}
	return u, nil
}

func confirmCode(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	code := c.Param("code")
	if code == "" {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "no code",
		})
	}
	ok, err := GetStore().GetCode(inst, code)
	if err != nil {
		inst.Logger().Warnf("Cannot get confirm code: %s", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	if !ok {
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": "invalid code",
		})
	}

	return c.NoContent(http.StatusNoContent)
}
