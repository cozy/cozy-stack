package auth

import (
	"encoding/hex"
	"net/http"

	"github.com/cozy/cozy-stack/model/instance"
	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/web/middlewares"
	"github.com/cozy/echo"
)

func passphraseResetForm(c echo.Context) error {
	instance := middlewares.GetInstance(c)
	if !instance.IsPasswordAuthenticationEnabled() {
		return c.Redirect(http.StatusSeeOther, instance.PageURL("/oidc/start", nil))
	}
	return c.Render(http.StatusOK, "passphrase_reset.html", echo.Map{
		"Title":       instance.TemplateTitle(),
		"CozyUI":      middlewares.CozyUI(instance),
		"ThemeCSS":    middlewares.ThemeCSS(instance),
		"Domain":      instance.ContextualDomain(),
		"ContextName": instance.ContextName,
		"Locale":      instance.Locale,
		"CSRF":        c.Get("csrf"),
	})
}

func passphraseForm(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	registerToken := c.QueryParams().Get("registerToken")
	if inst.OnboardingFinished {
		redirect := inst.DefaultRedirection()
		return c.Redirect(http.StatusSeeOther, redirect.String())
	}

	if registerToken == "" || !middlewares.CheckRegisterToken(c, inst) {
		return c.Render(http.StatusOK, "need_onboarding.html", echo.Map{
			"Title":       inst.TemplateTitle(),
			"ThemeCSS":    middlewares.ThemeCSS(inst),
			"Domain":      inst.ContextualDomain(),
			"ContextName": inst.ContextName,
			"Locale":      inst.Locale,
		})
	}

	matomo := config.GetConfig().Matomo
	if matomo.URL != "" {
		middlewares.AppendCSPRule(c, "img", matomo.URL)
	}
	return c.Render(http.StatusOK, "passphrase_onboarding.html", echo.Map{
		"CozyUI":        middlewares.CozyUI(inst),
		"Title":         inst.TemplateTitle(),
		"ThemeCSS":      middlewares.ThemeCSS(inst),
		"Domain":        inst.ContextualDomain(),
		"ContextName":   inst.ContextName,
		"Locale":        inst.Locale,
		"RegisterToken": registerToken,
		"MatomoURL":     matomo.URL,
		"MatomoSiteID":  matomo.SiteID,
		"MatomoAppID":   matomo.OnboardingAppID,
	})
}

func passphraseReset(c echo.Context) error {
	i := middlewares.GetInstance(c)
	if err := lifecycle.RequestPassphraseReset(i); err != nil && err != instance.ErrResetAlreadyRequested {
		return err
	}
	// Disconnect the user if it is logged in. The idea is that if the user
	// (maybe by accident) asks for a passphrase reset while logged in, we log
	// him out to be able to re-go through the process of logging back-in. It is
	// more a UX choice than a "security" one.
	session, ok := middlewares.GetSession(c)
	if ok {
		c.SetCookie(session.Delete(i))
	}
	return c.Render(http.StatusOK, "error.html", echo.Map{
		"Title":       i.TemplateTitle(),
		"CozyUI":      middlewares.CozyUI(i),
		"ThemeCSS":    middlewares.ThemeCSS(i),
		"Domain":      i.ContextualDomain(),
		"ContextName": i.ContextName,
		"ErrorTitle":  "Passphrase is reset Title",
		"Error":       "Passphrase is reset Body",
		"Button":      "Passphrase is reset Login Button",
		"ButtonLink":  i.PageURL("/auth/login", nil),
	})
}

func passphraseRenewForm(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if middlewares.IsLoggedIn(c) {
		redirect := inst.DefaultRedirection().String()
		return c.Redirect(http.StatusSeeOther, redirect)
	}
	// Check that the token is actually defined and well encoded. The actual
	// token value checking is also done on the passphraseRenew handler.
	token, err := hex.DecodeString(c.QueryParam("token"))
	if err != nil || len(token) == 0 {
		return renderError(c, http.StatusBadRequest, "Error Invalid reset token")
	}
	if err = lifecycle.CheckPassphraseRenewToken(inst, token); err != nil {
		if err == instance.ErrMissingToken {
			return renderError(c, http.StatusBadRequest, "Error Invalid reset token")
		}
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid_token",
		})
	}
	return c.Render(http.StatusOK, "passphrase_renew.html", echo.Map{
		"Title":                inst.TemplateTitle(),
		"CozyUI":               middlewares.CozyUI(inst),
		"ThemeCSS":             middlewares.ThemeCSS(inst),
		"Domain":               inst.ContextualDomain(),
		"ContextName":          inst.ContextName,
		"Locale":               inst.Locale,
		"PassphraseResetToken": hex.EncodeToString(token),
		"CSRF":                 c.Get("csrf"),
	})
}

func passphraseRenew(c echo.Context) error {
	inst := middlewares.GetInstance(c)
	if middlewares.IsLoggedIn(c) {
		redirect := inst.DefaultRedirection().String()
		return c.Redirect(http.StatusSeeOther, redirect)
	}
	pass := []byte(c.FormValue("passphrase"))
	token, err := hex.DecodeString(c.FormValue("passphrase_reset_token"))
	if err != nil {
		return renderError(c, http.StatusBadRequest, "Error Invalid reset token")
	}
	if err := lifecycle.PassphraseRenew(inst, pass, token); err != nil {
		if err == instance.ErrMissingToken {
			return renderError(c, http.StatusBadRequest, "Error Invalid reset token")
		}
		return c.JSON(http.StatusBadRequest, echo.Map{
			"error": "invalid_token",
		})
	}
	return c.Redirect(http.StatusSeeOther, inst.PageURL("/auth/login", nil))
}
