package auth

import (
	"errors"
	"net/http"

	"github.com/cozy/cozy-stack/model/bitwarden/settings"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/limits"
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
		return errors.New("Not yet supported") // TODO delegated auth
	}

	iterations := 0
	if settings, err := settings.Get(inst); err == nil {
		iterations = settings.PassphraseKdfIterations
	}
	return c.Render(http.StatusOK, "confirm_auth.html", echo.Map{
		"TemplateTitle":  inst.TemplateTitle(),
		"CozyUI":         middlewares.CozyUI(inst),
		"ThemeCSS":       middlewares.ThemeCSS(inst),
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
		return errors.New("Not yet supported") // TODO delegated auth
	}

	// Check passphrase
	passphrase := []byte(c.FormValue("passphrase"))
	if lifecycle.CheckPassphrase(inst, passphrase) != nil {
		errorMessage := inst.Translate(CredentialsErrorKey)
		err := limits.CheckRateLimit(inst, limits.AuthType)
		if limits.IsLimitReachedOrExceeded(err) {
			if err = LoginRateExceeded(inst); err != nil {
				inst.Logger().WithField("nspace", "auth").Warning(err)
			}
		}
		return c.JSON(http.StatusUnauthorized, echo.Map{
			"error": errorMessage,
		})
	}

	// TODO check 2fa
	// TODO send real-time event

	redirect, err := checkRedirectParam(c, inst.DefaultRedirection())
	if err != nil {
		return err
	}
	if wantsJSON(c) {
		return c.JSON(http.StatusOK, echo.Map{
			"redirect": redirect.String(),
		})
	}
	return c.Redirect(http.StatusSeeOther, redirect.String())
}
